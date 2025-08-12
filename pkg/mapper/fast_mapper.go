package mapper

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metalgrid/tr069-cel-mapper/pkg/extractor"
	"github.com/metalgrid/tr069-cel-mapper/pkg/pool"
	"github.com/metalgrid/tr069-cel-mapper/pkg/registry"
	"github.com/metalgrid/tr069-cel-mapper/pkg/router"
	"github.com/metalgrid/tr069-cel-mapper/pkg/transform"
	"github.com/metalgrid/tr069-cel-mapper/pkg/types"
)

type FastRule struct {
	ID        string
	Pattern   *router.Pattern
	Entity    string
	Field     string
	Transform string
	Extractor extractor.KeyExtractor
}

type FastMapper struct {
	router      *router.FastRouter
	rules       map[string]*FastRule
	registry    *registry.Registry
	store       types.Store
	objectPool  *pool.ObjectPool
	transformer *transform.FastTransform

	stats        *FastStats
	errorHandler func(error)

	mu sync.RWMutex
}

type FastStats struct {
	ProcessedLines  atomic.Int64
	MatchedRules    atomic.Int64
	FailedRules     atomic.Int64
	CacheHits       atomic.Int64
	CacheMisses     atomic.Int64
	AllocCount      atomic.Int64
	ReuseCount      atomic.Int64
	ProcessingNanos atomic.Int64
}

type FastOption func(*FastMapper)

func WithFastStats() FastOption {
	return func(m *FastMapper) {
		m.stats = &FastStats{}
	}
}

func WithFastErrorHandler(handler func(error)) FastOption {
	return func(m *FastMapper) {
		m.errorHandler = handler
	}
}

func NewFast(reg *registry.Registry, opts ...FastOption) *FastMapper {
	m := &FastMapper{
		router:       router.New(),
		rules:        make(map[string]*FastRule),
		registry:     reg,
		store:        types.NewMapStore(),
		objectPool:   pool.New(),
		transformer:  transform.NewFastTransform(),
		errorHandler: func(err error) {},
	}

	for _, opt := range opts {
		opt(m)
	}

	for _, typeName := range reg.List() {
		info, _ := reg.Get(typeName)
		m.objectPool.Register(typeName, info.Factory)
	}

	return m
}

func (m *FastMapper) AddRule(rule *FastRule) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule.Pattern.ID = rule.ID
	m.router.AddPattern(rule.Pattern)
	m.rules[rule.ID] = rule
}

func (m *FastMapper) Process(path, value string) error {
	return m.ProcessContext(context.Background(), path, value)
}

func (m *FastMapper) ProcessContext(ctx context.Context, path, value string) error {
	start := time.Now()

	pattern, matched := m.router.Route(path)
	if !matched {
		if m.stats != nil {
			m.stats.CacheMisses.Add(1)
		}
		return nil
	}

	if m.stats != nil {
		m.stats.MatchedRules.Add(1)
		defer func() {
			m.stats.ProcessedLines.Add(1)
			m.stats.ProcessingNanos.Add(time.Since(start).Nanoseconds())
		}()
	}

	rule, ok := m.rules[pattern.ID]
	if !ok {
		return fmt.Errorf("rule not found: %s", pattern.ID)
	}

	key := rule.Extractor.Extract(path, value)

	var obj any
	if m.objectPool != nil {
		if pooled, ok := m.objectPool.Get(rule.Entity); ok {
			obj = pooled
			if m.stats != nil {
				m.stats.ReuseCount.Add(1)
			}
		}
	}

	if obj == nil {
		info, err := m.registry.Get(rule.Entity)
		if err != nil {
			return err
		}
		obj = info.Factory()
		if m.stats != nil {
			m.stats.AllocCount.Add(1)
		}
	}

	existing := m.store.Upsert(rule.Entity, key, func() any {
		return obj
	})

	if existing != obj && m.objectPool != nil {
		m.objectPool.Put(rule.Entity, obj)
		obj = existing
	}

	var finalValue any = value
	if rule.Transform != "" {
		transformed, err := m.transformer.Transform(rule.Transform, value)
		if err != nil {
			if m.stats != nil {
				m.stats.FailedRules.Add(1)
			}
			m.errorHandler(fmt.Errorf("transform failed: %w", err))
			return nil
		}
		finalValue = transformed
	}

	info, _ := m.registry.Get(rule.Entity)
	if setter, ok := info.Setters[rule.Field]; ok {
		if err := setter(obj, finalValue); err != nil {
			if m.stats != nil {
				m.stats.FailedRules.Add(1)
			}
			m.errorHandler(fmt.Errorf("setter failed: %w", err))
		}
	}

	return nil
}

func (m *FastMapper) ProcessBatch(items [][2]string) error {
	return m.ProcessBatchContext(context.Background(), items)
}

func (m *FastMapper) ProcessBatchContext(ctx context.Context, items [][2]string) error {
	const batchSize = 100

	if len(items) < batchSize*2 {
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := m.ProcessContext(ctx, item[0], item[1]); err != nil {
				return err
			}
		}
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	numWorkers := (len(items) + batchSize - 1) / batchSize
	if numWorkers > 10 {
		numWorkers = 10
	}

	itemsChan := make(chan [2]string, len(items))
	for _, item := range items {
		itemsChan <- item
	}
	close(itemsChan)

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for item := range itemsChan {
				if err := ctx.Err(); err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}
				if err := m.ProcessContext(ctx, item[0], item[1]); err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

func (m *FastMapper) GetStore() types.Store {
	return m.store
}

func (m *FastMapper) GetStats() *FastStats {
	return m.stats
}

func (m *FastMapper) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.store.Clear()
	if m.stats != nil {
		m.stats.ProcessedLines.Store(0)
		m.stats.MatchedRules.Store(0)
		m.stats.FailedRules.Store(0)
		m.stats.CacheHits.Store(0)
		m.stats.CacheMisses.Store(0)
		m.stats.AllocCount.Store(0)
		m.stats.ReuseCount.Store(0)
		m.stats.ProcessingNanos.Store(0)
	}
}

func (s *FastStats) String() string {
	if s == nil {
		return "Stats: disabled"
	}

	processed := s.ProcessedLines.Load()
	if processed == 0 {
		return "Stats: no data processed"
	}

	nanos := s.ProcessingNanos.Load()
	avgNanos := nanos / processed

	return fmt.Sprintf(
		"Stats: %d lines, %d matched, %d failed | "+
			"Cache: %d hits, %d misses (%.1f%% hit rate) | "+
			"Memory: %d allocs, %d reused (%.1f%% reuse rate) | "+
			"Avg latency: %dns",
		processed, s.MatchedRules.Load(), s.FailedRules.Load(),
		s.CacheHits.Load(), s.CacheMisses.Load(),
		float64(s.CacheHits.Load())*100/float64(s.CacheHits.Load()+s.CacheMisses.Load()+1),
		s.AllocCount.Load(), s.ReuseCount.Load(),
		float64(s.ReuseCount.Load())*100/float64(s.AllocCount.Load()+s.ReuseCount.Load()+1),
		avgNanos,
	)
}

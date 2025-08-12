package mapper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/example/cel-mapper/pkg/builder"
	"github.com/example/cel-mapper/pkg/registry"
	"github.com/example/cel-mapper/pkg/types"
)

type Mapper struct {
	rules    []*types.CompiledRule
	registry *registry.Registry
	store    types.Store
	mu       sync.RWMutex

	errorHandler func(error)
	metrics      *Metrics
}

type Metrics struct {
	mu              sync.RWMutex
	ProcessedLines  int64
	MatchedRules    int64
	FailedRules     int64
	ProcessingTime  time.Duration
	LastProcessTime time.Time
}

type Option func(*Mapper)

func WithStore(store types.Store) Option {
	return func(m *Mapper) {
		m.store = store
	}
}

func WithErrorHandler(handler func(error)) Option {
	return func(m *Mapper) {
		m.errorHandler = handler
	}
}

func WithMetrics() Option {
	return func(m *Mapper) {
		m.metrics = &Metrics{}
	}
}

func New(reg *registry.Registry, opts ...Option) *Mapper {
	m := &Mapper{
		registry: reg,
		store:    types.NewMapStore(),
		errorHandler: func(err error) {
		},
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

func (m *Mapper) LoadRules(rules []*types.CompiledRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rule := range rules {
		if !m.registry.Has(rule.Target) {
			return fmt.Errorf("rule %s: target type %s not registered", rule.Name, rule.Target)
		}
	}

	m.rules = rules
	return nil
}

func (m *Mapper) LoadRulesFromFile(filename string) error {
	builder := builder.New(m.registry).WithStandardVariables()
	rules, err := builder.BuildFromFile(filename)
	if err != nil {
		return err
	}
	return m.LoadRules(rules)
}

func (m *Mapper) LoadRulesFromString(content string) error {
	builder := builder.New(m.registry).WithStandardVariables()
	rules, err := builder.BuildFromString(content)
	if err != nil {
		return err
	}
	return m.LoadRules(rules)
}

func (m *Mapper) Process(path, value string) error {
	return m.ProcessWithContext(context.Background(), path, value)
}

func (m *Mapper) ProcessWithContext(ctx context.Context, path, value string) error {
	start := time.Now()
	defer func() {
		if m.metrics != nil {
			m.metrics.mu.Lock()
			m.metrics.ProcessedLines++
			m.metrics.ProcessingTime += time.Since(start)
			m.metrics.LastProcessTime = time.Now()
			m.metrics.mu.Unlock()
		}
	}()

	m.mu.RLock()
	rules := m.rules
	m.mu.RUnlock()

	processCtx := types.NewProcessContext(path, value)

	for _, rule := range rules {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		matched, err := m.applyRule(rule, processCtx)
		if err != nil {
			if m.metrics != nil {
				m.metrics.mu.Lock()
				m.metrics.FailedRules++
				m.metrics.mu.Unlock()
			}
			m.errorHandler(fmt.Errorf("rule %s: %w", rule.Name, err))
			continue
		}

		if matched {
			if m.metrics != nil {
				m.metrics.mu.Lock()
				m.metrics.MatchedRules++
				m.metrics.mu.Unlock()
			}
			return nil
		}
	}

	return nil
}

func (m *Mapper) applyRule(rule *types.CompiledRule, ctx *types.ProcessContext) (bool, error) {
	routeVal, _, err := rule.Route.Eval(ctx.Data)
	if err != nil {
		return false, fmt.Errorf("route evaluation failed: %w", err)
	}

	matched, ok := routeVal.Value().(bool)
	if !ok {
		return false, fmt.Errorf("route expression must return boolean, got %T", routeVal.Value())
	}

	if !matched {
		return false, nil
	}

	keyVal, _, err := rule.EntityKey.Eval(ctx.Data)
	if err != nil {
		return false, fmt.Errorf("entity key evaluation failed: %w", err)
	}

	key, ok := keyVal.Value().(string)
	if !ok {
		return false, fmt.Errorf("entity key must return string, got %T", keyVal.Value())
	}

	obj := m.store.Upsert(rule.Target, key, rule.Factory)

	for _, field := range rule.Fields {
		if err := m.applyField(field, ctx, obj); err != nil {
			return false, fmt.Errorf("field %s: %w", field.Name, err)
		}
	}

	return true, nil
}

func (m *Mapper) applyField(field types.CompiledFieldRule, ctx *types.ProcessContext, obj any) error {
	whenVal, _, err := field.When.Eval(ctx.Data)
	if err != nil {
		return fmt.Errorf("when evaluation failed: %w", err)
	}

	shouldApply, ok := whenVal.Value().(bool)
	if !ok {
		return fmt.Errorf("when expression must return boolean, got %T", whenVal.Value())
	}

	if !shouldApply {
		return nil
	}

	valueVal, _, err := field.Value.Eval(ctx.Data)
	if err != nil {
		return fmt.Errorf("value evaluation failed: %w", err)
	}

	if err := field.Setter(obj, valueVal.Value()); err != nil {
		return fmt.Errorf("setter failed: %w", err)
	}

	return nil
}

func (m *Mapper) ProcessBatch(items [][2]string) error {
	return m.ProcessBatchWithContext(context.Background(), items)
}

func (m *Mapper) ProcessBatchWithContext(ctx context.Context, items [][2]string) error {
	for _, item := range items {
		if err := m.ProcessWithContext(ctx, item[0], item[1]); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mapper) GetStore() types.Store {
	return m.store
}

func (m *Mapper) GetMetrics() *Metrics {
	return m.metrics
}

func (m *Mapper) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.store.Clear()
	if m.metrics != nil {
		m.metrics.mu.Lock()
		m.metrics.ProcessedLines = 0
		m.metrics.MatchedRules = 0
		m.metrics.FailedRules = 0
		m.metrics.ProcessingTime = 0
		m.metrics.mu.Unlock()
	}
}

func (m *Mapper) GetRuleNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, len(m.rules))
	for i, rule := range m.rules {
		names[i] = rule.Name
	}
	return names
}

package types

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/google/cel-go/cel"
)

type FieldMapping struct {
	Name      string `yaml:"name"`
	When      string `yaml:"when"`
	Value     string `yaml:"value"`
	FieldType string `yaml:"type,omitempty"`
}

type RuleConfig struct {
	Name      string         `yaml:"name"`
	Target    string         `yaml:"target"`
	Route     string         `yaml:"route"`
	EntityKey string         `yaml:"entity_key"`
	Fields    []FieldMapping `yaml:"fields"`
}

type RulesConfig struct {
	Version string       `yaml:"version"`
	Rules   []RuleConfig `yaml:"rules"`
}

type CompiledFieldRule struct {
	Name      string
	When      cel.Program
	Value     cel.Program
	FieldType reflect.Type
	Setter    func(any, any) error
}

type CompiledRule struct {
	Name      string
	Target    string
	Route     cel.Program
	EntityKey cel.Program
	Fields    []CompiledFieldRule
	Factory   func() any
}

type ProcessContext struct {
	Path  string
	Value string
	Data  map[string]any
}

func NewProcessContext(path, value string) *ProcessContext {
	return &ProcessContext{
		Path:  path,
		Value: value,
		Data:  map[string]any{"path": path, "value": value},
	}
}

func (ctx *ProcessContext) WithData(key string, value any) *ProcessContext {
	ctx.Data[key] = value
	return ctx
}

type Store interface {
	Upsert(target, key string, factory func() any) any
	Get(target, key string) (any, bool)
	GetAll(target string) map[string]any
	ForEach(fn func(target, key string, obj any) error) error
	Clear()
}

type MapStore struct {
	mu   sync.RWMutex
	data map[string]map[string]any
}

func NewMapStore() *MapStore {
	return &MapStore{
		data: make(map[string]map[string]any),
	}
}

func (s *MapStore) Upsert(target, key string, factory func() any) any {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.data[target]
	if !ok {
		group = make(map[string]any)
		s.data[target] = group
	}

	obj, ok := group[key]
	if !ok {
		obj = factory()
		group[key] = obj
	}
	return obj
}

func (s *MapStore) Get(target, key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.data[target]
	if !ok {
		return nil, false
	}
	obj, ok := group[key]
	return obj, ok
}

func (s *MapStore) GetAll(target string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.data[target]
	if !ok {
		return nil
	}
	result := make(map[string]any, len(group))
	for k, v := range group {
		result[k] = v
	}
	return result
}

func (s *MapStore) ForEach(fn func(target, key string, obj any) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for target, group := range s.data {
		for key, obj := range group {
			if err := fn(target, key, obj); err != nil {
				return fmt.Errorf("error processing %s[%s]: %w", target, key, err)
			}
		}
	}
	return nil
}

func (s *MapStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = make(map[string]map[string]any)
}

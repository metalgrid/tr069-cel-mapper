package pool

import (
	"reflect"
	"strings"
	"sync"
)

type ObjectPool struct {
	pools map[string]*sync.Pool
	mu    sync.RWMutex
}

func New() *ObjectPool {
	return &ObjectPool{
		pools: make(map[string]*sync.Pool),
	}
}

func (p *ObjectPool) Register(typeName string, factory func() any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.pools[typeName] = &sync.Pool{
		New: factory,
	}
}

func (p *ObjectPool) Get(typeName string) (any, bool) {
	p.mu.RLock()
	pool, ok := p.pools[typeName]
	p.mu.RUnlock()

	if !ok {
		return nil, false
	}

	return pool.Get(), true
}

func (p *ObjectPool) Put(typeName string, obj any) {
	p.mu.RLock()
	pool, ok := p.pools[typeName]
	p.mu.RUnlock()

	if !ok {
		return
	}

	p.resetObject(obj)
	pool.Put(obj)
}

func (p *ObjectPool) resetObject(obj any) {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}
		field.Set(reflect.Zero(field.Type()))
	}
}

type BufferPool struct {
	pool *sync.Pool
}

func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		pool: &sync.Pool{
			New: func() any {
				return make([]byte, 0, size)
			},
		},
	}
}

func (b *BufferPool) Get() []byte {
	buf := b.pool.Get().([]byte)
	return buf[:0]
}

func (b *BufferPool) Put(buf []byte) {
	if cap(buf) > 0 {
		buf = buf[:0]
		b.pool.Put(buf)
	}
}

type StringBuilderPool struct {
	pool *sync.Pool
}

func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: &sync.Pool{
			New: func() any {
				return new(strings.Builder)
			},
		},
	}
}

func (s *StringBuilderPool) Get() *strings.Builder {
	sb := s.pool.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

func (s *StringBuilderPool) Put(sb *strings.Builder) {
	sb.Reset()
	s.pool.Put(sb)
}

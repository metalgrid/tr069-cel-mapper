package registry

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
)

type TypeInfo struct {
	Type    reflect.Type
	Factory func() any
	Setters map[string]func(any, any) error
}

type Registry struct {
	mu    sync.RWMutex
	types map[string]*TypeInfo
}

func New() *Registry {
	return &Registry{
		types: make(map[string]*TypeInfo),
	}
}

func (r *Registry) Register(name string, factory func() any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.types[name]; exists {
		return fmt.Errorf("type %s already registered", name)
	}

	obj := factory()
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	setters, err := buildSetters(t)
	if err != nil {
		return fmt.Errorf("failed to build setters for %s: %w", name, err)
	}

	r.types[name] = &TypeInfo{
		Type:    t,
		Factory: factory,
		Setters: setters,
	}

	return nil
}

func (r *Registry) MustRegister(name string, factory func() any) {
	if err := r.Register(name, factory); err != nil {
		panic(err)
	}
}

func (r *Registry) Get(name string) (*TypeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.types[name]
	if !ok {
		return nil, fmt.Errorf("type %s not registered", name)
	}
	return info, nil
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[name]
	return ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.types))
	for name := range r.types {
		names = append(names, name)
	}
	return names
}

func buildSetters(t reflect.Type) (map[string]func(any, any) error, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct type, got %s", t.Kind())
	}

	setters := make(map[string]func(any, any) error)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldIndex := i
		fieldName := field.Name
		fieldType := field.Type

		setters[fieldName] = func(obj any, value any) error {
			rv := reflect.ValueOf(obj)
			if rv.Kind() == reflect.Ptr {
				rv = rv.Elem()
			}

			if !rv.IsValid() || rv.Kind() != reflect.Struct {
				return fmt.Errorf("invalid object for field %s", fieldName)
			}

			fieldValue := rv.Field(fieldIndex)
			if !fieldValue.CanSet() {
				return fmt.Errorf("cannot set field %s", fieldName)
			}

			return setFieldValue(fieldValue, fieldType, value, fieldName)
		}

		if tag := field.Tag.Get("json"); tag != "" {
			setters[tag] = setters[fieldName]
		}
		if tag := field.Tag.Get("yaml"); tag != "" {
			setters[tag] = setters[fieldName]
		}
	}

	return setters, nil
}

func setFieldValue(fieldValue reflect.Value, fieldType reflect.Type, value any, fieldName string) error {
	if value == nil {
		if fieldType.Kind() == reflect.Ptr {
			fieldValue.Set(reflect.Zero(fieldType))
			return nil
		}
		return fmt.Errorf("cannot set nil to non-pointer field %s", fieldName)
	}

	valueType := reflect.TypeOf(value)

	if fieldType.Kind() == reflect.Ptr {
		if valueType == fieldType.Elem() {
			ptr := reflect.New(fieldType.Elem())
			ptr.Elem().Set(reflect.ValueOf(value))
			fieldValue.Set(ptr)
			return nil
		}
	}

	if valueType.AssignableTo(fieldType) {
		fieldValue.Set(reflect.ValueOf(value))
		return nil
	}

	switch fieldType.Kind() {
	case reflect.String:
		str, err := toString(value)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldName, err)
		}
		fieldValue.SetString(str)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := toInt64(value)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldName, err)
		}
		if fieldValue.OverflowInt(i) {
			return fmt.Errorf("field %s: integer overflow", fieldName)
		}
		fieldValue.SetInt(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := toUint64(value)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldName, err)
		}
		if fieldValue.OverflowUint(u) {
			return fmt.Errorf("field %s: unsigned integer overflow", fieldName)
		}
		fieldValue.SetUint(u)

	case reflect.Float32, reflect.Float64:
		f, err := toFloat64(value)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldName, err)
		}
		if fieldValue.OverflowFloat(f) {
			return fmt.Errorf("field %s: float overflow", fieldName)
		}
		fieldValue.SetFloat(f)

	case reflect.Bool:
		b, err := toBool(value)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldName, err)
		}
		fieldValue.SetBool(b)

	case reflect.Slice:
		if err := setSliceValue(fieldValue, fieldType, value, fieldName); err != nil {
			return err
		}

	case reflect.Map:
		if err := setMapValue(fieldValue, fieldType, value, fieldName); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported field type %s for field %s", fieldType.Kind(), fieldName)
	}

	return nil
}

func setSliceValue(fieldValue reflect.Value, fieldType reflect.Type, value any, fieldName string) error {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return fmt.Errorf("field %s: expected slice or array, got %T", fieldName, value)
	}

	elemType := fieldType.Elem()
	slice := reflect.MakeSlice(fieldType, rv.Len(), rv.Len())

	for i := 0; i < rv.Len(); i++ {
		elem := reflect.New(elemType).Elem()
		if err := setFieldValue(elem, elemType, rv.Index(i).Interface(), fmt.Sprintf("%s[%d]", fieldName, i)); err != nil {
			return err
		}
		slice.Index(i).Set(elem)
	}

	fieldValue.Set(slice)
	return nil
}

func setMapValue(fieldValue reflect.Value, fieldType reflect.Type, value any, fieldName string) error {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map {
		return fmt.Errorf("field %s: expected map, got %T", fieldName, value)
	}

	keyType := fieldType.Key()
	elemType := fieldType.Elem()
	mapValue := reflect.MakeMap(fieldType)

	for _, key := range rv.MapKeys() {
		k := reflect.New(keyType).Elem()
		if err := setFieldValue(k, keyType, key.Interface(), fmt.Sprintf("%s.key", fieldName)); err != nil {
			return err
		}

		v := reflect.New(elemType).Elem()
		if err := setFieldValue(v, elemType, rv.MapIndex(key).Interface(), fmt.Sprintf("%s[%v]", fieldName, key.Interface())); err != nil {
			return err
		}

		mapValue.SetMapIndex(k, v)
	}

	fieldValue.Set(mapValue)
	return nil
}

func toString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case []byte:
		return string(x), nil
	case fmt.Stringer:
		return x.String(), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case uint64:
		return int64(x), nil
	case uint:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case float64:
		return int64(x), nil
	case float32:
		return int64(x), nil
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

func toUint64(v any) (uint64, error) {
	switch x := v.(type) {
	case uint64:
		return x, nil
	case uint:
		return uint64(x), nil
	case uint32:
		return uint64(x), nil
	case uint16:
		return uint64(x), nil
	case uint8:
		return uint64(x), nil
	case int64:
		if x < 0 {
			return 0, fmt.Errorf("cannot convert negative int64 to uint64")
		}
		return uint64(x), nil
	case int:
		if x < 0 {
			return 0, fmt.Errorf("cannot convert negative int to uint64")
		}
		return uint64(x), nil
	case float64:
		if x < 0 {
			return 0, fmt.Errorf("cannot convert negative float64 to uint64")
		}
		return uint64(x), nil
	case string:
		return strconv.ParseUint(x, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to uint64", v)
	}
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case int:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case uint:
		return float64(x), nil
	case string:
		return strconv.ParseFloat(x, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		return strconv.ParseBool(x)
	case int, int64, int32, int16, int8:
		return reflect.ValueOf(x).Int() != 0, nil
	case uint, uint64, uint32, uint16, uint8:
		return reflect.ValueOf(x).Uint() != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

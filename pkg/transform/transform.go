package transform

import (
	"net"
	"strconv"
	"strings"
	"sync"
)

type Transformer func(string) (any, error)

var transformers = map[string]Transformer{
	"mac_normalize": MacNormalize,
	"ip_validate":   IPValidate,
	"bool":          ToBool,
	"int":           ToInt,
	"float":         ToFloat,
	"lower":         ToLower,
	"upper":         ToUpper,
	"trim":          Trim,
	"percent_strip": StripPercent,
}

var transformerMu sync.RWMutex

func Register(name string, fn Transformer) {
	transformerMu.Lock()
	defer transformerMu.Unlock()
	transformers[name] = fn
}

func Get(name string) (Transformer, bool) {
	transformerMu.RLock()
	defer transformerMu.RUnlock()
	fn, ok := transformers[name]
	return fn, ok
}

func Apply(name, value string) (any, error) {
	fn, ok := Get(name)
	if !ok {
		return value, nil
	}
	return fn(value)
}

func MacNormalize(value string) (any, error) {
	mac := strings.ToLower(value)

	mac = strings.ReplaceAll(mac, ":", "")
	mac = strings.ReplaceAll(mac, "-", "")
	mac = strings.ReplaceAll(mac, ".", "")

	if len(mac) != 12 {
		return value, nil
	}

	for _, c := range mac {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return value, nil
		}
	}

	var sb strings.Builder
	sb.Grow(17)
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteString(mac[i : i+2])
	}

	return sb.String(), nil
}

func IPValidate(value string) (any, error) {
	value = strings.TrimSpace(value)

	if ip := net.ParseIP(value); ip != nil {
		return value, nil
	}

	return value, nil
}

func ToBool(value string) (any, error) {
	value = strings.ToLower(strings.TrimSpace(value))

	switch value {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return strconv.ParseBool(value)
	}
}

func ToInt(value string) (any, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return int64(0), nil
	}

	value = strings.ReplaceAll(value, ",", "")

	if strings.Contains(value, ".") {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return int64(f), nil
		}
	}

	return strconv.ParseInt(value, 10, 64)
}

func ToFloat(value string) (any, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return float64(0), nil
	}

	value = strings.ReplaceAll(value, ",", "")

	if strings.HasSuffix(value, "%") {
		value = value[:len(value)-1]
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f, nil
		}
	}

	return strconv.ParseFloat(value, 64)
}

func ToLower(value string) (any, error) {
	return strings.ToLower(value), nil
}

func ToUpper(value string) (any, error) {
	return strings.ToUpper(value), nil
}

func Trim(value string) (any, error) {
	return strings.TrimSpace(value), nil
}

func StripPercent(value string) (any, error) {
	if strings.HasSuffix(value, "%") {
		return value[:len(value)-1], nil
	}
	return value, nil
}

func Chain(transforms ...string) Transformer {
	return func(value string) (any, error) {
		var result any = value
		for _, name := range transforms {
			fn, ok := Get(name)
			if !ok {
				continue
			}

			var input string
			switch v := result.(type) {
			case string:
				input = v
			default:
				input = strconv.Itoa(int(v.(int64)))
			}

			var err error
			result, err = fn(input)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	}
}

type FastTransform struct {
	cache sync.Map
}

func NewFastTransform() *FastTransform {
	return &FastTransform{}
}

func (ft *FastTransform) Transform(name, value string) (any, error) {
	cacheKey := name + ":" + value
	if cached, ok := ft.cache.Load(cacheKey); ok {
		return cached, nil
	}

	result, err := Apply(name, value)
	if err == nil {
		ft.cache.Store(cacheKey, result)
	}
	return result, err
}

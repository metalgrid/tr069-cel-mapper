package extractor

import (
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

type KeyExtractor interface {
	Extract(path, value string) string
}

type IndexExtractor struct {
	Position int
	Prefix   string
}

func (e *IndexExtractor) Extract(path, value string) string {
	parts := splitPathCached(path)
	if e.Position < 0 || e.Position >= len(parts) {
		return ""
	}
	if e.Prefix != "" {
		return e.Prefix + parts[e.Position]
	}
	return parts[e.Position]
}

type ValueExtractor struct{}

func (e *ValueExtractor) Extract(path, value string) string {
	return value
}

type CompositeExtractor struct {
	Parts []KeyExtractor
	Sep   string
}

func (e *CompositeExtractor) Extract(path, value string) string {
	if len(e.Parts) == 0 {
		return ""
	}
	if len(e.Parts) == 1 {
		return e.Parts[0].Extract(path, value)
	}

	sb := getStringBuilder()
	defer putStringBuilder(sb)

	for i, part := range e.Parts {
		if i > 0 && e.Sep != "" {
			sb.WriteString(e.Sep)
		}
		sb.WriteString(part.Extract(path, value))
	}
	return sb.String()
}

type StaticExtractor struct {
	Value string
}

func (e *StaticExtractor) Extract(path, value string) string {
	return e.Value
}

type LastPartExtractor struct {
	Count int
}

func (e *LastPartExtractor) Extract(path, value string) string {
	lastDot := -1
	count := 0
	for i := len(path) - 1; i >= 0 && count < e.Count; i-- {
		if path[i] == '.' {
			count++
			if count == e.Count {
				lastDot = i
				break
			}
		}
	}
	if lastDot >= 0 && lastDot < len(path)-1 {
		return path[lastDot+1:]
	}
	return path
}

func CompileExtractor(pattern string) KeyExtractor {
	if pattern == "value" {
		return &ValueExtractor{}
	}

	if strings.HasPrefix(pattern, "path[") && strings.HasSuffix(pattern, "]") {
		idxStr := pattern[5 : len(pattern)-1]
		if idx, err := strconv.Atoi(idxStr); err == nil {
			return &IndexExtractor{Position: idx}
		}
	}

	if strings.Contains(pattern, "+") {
		parts := strings.Split(pattern, "+")
		extractors := make([]KeyExtractor, len(parts))
		for i, part := range parts {
			extractors[i] = CompileExtractor(strings.TrimSpace(part))
		}
		return &CompositeExtractor{Parts: extractors, Sep: ""}
	}

	if strings.Contains(pattern, ":") {
		parts := strings.Split(pattern, ":")
		extractors := make([]KeyExtractor, len(parts))
		for i, part := range parts {
			extractors[i] = CompileExtractor(strings.TrimSpace(part))
		}
		return &CompositeExtractor{Parts: extractors, Sep: ":"}
	}

	return &StaticExtractor{Value: pattern}
}

var pathCache = &sync.Map{}

func splitPathCached(path string) []string {
	if cached, ok := pathCache.Load(path); ok {
		return cached.([]string)
	}

	parts := splitPathFast(path)
	pathCache.Store(path, parts)
	return parts
}

func splitPathFast(path string) []string {
	n := 1
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			n++
		}
	}

	parts := make([]string, 0, n)
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}

func ExtractInstance(path string, after string) string {
	idx := strings.Index(path, after)
	if idx < 0 {
		return ""
	}

	start := idx + len(after)
	if start >= len(path) {
		return ""
	}

	if path[start] == '.' {
		start++
	}

	end := start
	for end < len(path) && path[end] != '.' {
		end++
	}

	if end > start {
		return path[start:end]
	}
	return ""
}

func ExtractBetween(path, prefix, suffix string) string {
	start := strings.Index(path, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)

	end := strings.Index(path[start:], suffix)
	if end < 0 {
		return ""
	}

	return path[start : start+end]
}

func UnsafeStringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func UnsafeBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

var sbPool = &sync.Pool{
	New: func() any {
		return new(strings.Builder)
	},
}

func getStringBuilder() *strings.Builder {
	sb := sbPool.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

func putStringBuilder(sb *strings.Builder) {
	sb.Reset()
	sbPool.Put(sb)
}

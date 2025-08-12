package router

import (
	"strings"
	"sync"
	"unsafe"
)

type Pattern struct {
	ID           string
	OriginalPath string
	Prefix       string
	Suffix       string
	Contains     []string
	Parts        []string
	MinParts     int
	MaxParts     int
	WildcardPos  []int
	Entity       string
	Field        string
	Priority     int
}

type FastRouter struct {
	exactMatches map[string]*Pattern
	prefixTree   *Trie
	suffixIndex  map[string][]*Pattern
	patterns     []*Pattern
	mu           sync.RWMutex
}

func New() *FastRouter {
	return &FastRouter{
		exactMatches: make(map[string]*Pattern),
		prefixTree:   NewTrie(),
		suffixIndex:  make(map[string][]*Pattern),
		patterns:     make([]*Pattern, 0, 256),
	}
}

func (r *FastRouter) AddPattern(p *Pattern) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.Prefix != "" && p.WildcardPos == nil {
		r.exactMatches[p.OriginalPath] = p
		return
	}

	if p.Prefix != "" && len(p.WildcardPos) > 0 {
		r.prefixTree.Insert(p.Prefix, p)
	}

	if p.Suffix != "" {
		r.suffixIndex[p.Suffix] = append(r.suffixIndex[p.Suffix], p)
	}

	r.patterns = append(r.patterns, p)
}

func (r *FastRouter) Route(path string) (*Pattern, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if pattern, ok := r.exactMatches[path]; ok {
		return pattern, true
	}

	pathLen := len(path)
	pathBytes := unsafeStringToBytes(path)

	if patterns := r.prefixTree.Search(path); len(patterns) > 0 {
		for _, p := range patterns {
			if r.matchPatternFast(pathBytes, pathLen, p) {
				return p, true
			}
		}
	}

	lastDot := strings.LastIndexByte(path, '.')
	if lastDot > 0 {
		suffix := path[lastDot:]
		if patterns, ok := r.suffixIndex[suffix]; ok {
			for _, p := range patterns {
				if r.matchPatternFast(pathBytes, pathLen, p) {
					return p, true
				}
			}
		}
	}

	for _, p := range r.patterns {
		if r.matchPatternFast(pathBytes, pathLen, p) {
			return p, true
		}
	}

	return nil, false
}

func (r *FastRouter) matchPatternFast(pathBytes []byte, pathLen int, p *Pattern) bool {
	if p.Prefix != "" {
		prefixLen := len(p.Prefix)
		if pathLen < prefixLen || !bytesHasPrefix(pathBytes, p.Prefix) {
			return false
		}
	}

	if p.Suffix != "" {
		suffixLen := len(p.Suffix)
		if pathLen < suffixLen || !bytesHasSuffix(pathBytes, pathLen, p.Suffix) {
			return false
		}
	}

	if len(p.Contains) > 0 {
		for _, contains := range p.Contains {
			if !bytesContains(pathBytes, pathLen, contains) {
				return false
			}
		}
	}

	if len(p.Parts) > 0 {
		return r.matchParts(string(pathBytes[:pathLen]), p)
	}

	if p.MinParts > 0 || p.MaxParts > 0 {
		partCount := countDots(pathBytes, pathLen) + 1
		if p.MinParts > 0 && partCount < p.MinParts {
			return false
		}
		if p.MaxParts > 0 && partCount > p.MaxParts {
			return false
		}
	}

	return true
}

func (r *FastRouter) matchParts(path string, p *Pattern) bool {
	parts := splitPathFast(path)

	if len(parts) != len(p.Parts) {
		return false
	}

	for i, expectedPart := range p.Parts {
		if expectedPart != "*" && expectedPart != parts[i] {
			return false
		}
	}

	return true
}

func CompilePattern(path string) *Pattern {
	p := &Pattern{
		OriginalPath: path,
		Priority:     0,
	}

	if !strings.Contains(path, "*") {
		p.Prefix = path
		return p
	}

	parts := strings.Split(path, ".")
	p.Parts = parts
	p.MinParts = len(parts)
	p.MaxParts = len(parts)

	wildcardPos := make([]int, 0)
	for i, part := range parts {
		if part == "*" {
			wildcardPos = append(wildcardPos, i)
		}
	}
	p.WildcardPos = wildcardPos

	firstWildcard := -1
	for i, part := range parts {
		if part == "*" {
			firstWildcard = i
			break
		}
	}

	if firstWildcard > 0 {
		p.Prefix = strings.Join(parts[:firstWildcard], ".") + "."
	}

	lastWildcard := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "*" {
			lastWildcard = i
			break
		}
	}

	if lastWildcard >= 0 && lastWildcard < len(parts)-1 {
		p.Suffix = "." + strings.Join(parts[lastWildcard+1:], ".")
	}

	return p
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
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}

func bytesHasPrefix(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if b[i] != prefix[i] {
			return false
		}
	}
	return true
}

func bytesHasSuffix(b []byte, bLen int, suffix string) bool {
	sLen := len(suffix)
	if bLen < sLen {
		return false
	}
	offset := bLen - sLen
	for i := 0; i < sLen; i++ {
		if b[offset+i] != suffix[i] {
			return false
		}
	}
	return true
}

func bytesContains(b []byte, bLen int, substr string) bool {
	subLen := len(substr)
	if bLen < subLen {
		return false
	}
	if subLen == 0 {
		return true
	}

	first := substr[0]
	for i := 0; i <= bLen-subLen; i++ {
		if b[i] != first {
			continue
		}
		match := true
		for j := 1; j < subLen; j++ {
			if b[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func countDots(b []byte, length int) int {
	count := 0
	for i := 0; i < length; i++ {
		if b[i] == '.' {
			count++
		}
	}
	return count
}

func unsafeStringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

package router

import (
	"sync"
)

type TrieNode struct {
	children map[byte]*TrieNode
	patterns []*Pattern
	isEnd    bool
}

type Trie struct {
	root *TrieNode
	mu   sync.RWMutex
}

func NewTrie() *Trie {
	return &Trie{
		root: &TrieNode{
			children: make(map[byte]*TrieNode),
			patterns: make([]*Pattern, 0),
		},
	}
}

func (t *Trie) Insert(prefix string, pattern *Pattern) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	for i := 0; i < len(prefix); i++ {
		char := prefix[i]
		if _, ok := node.children[char]; !ok {
			node.children[char] = &TrieNode{
				children: make(map[byte]*TrieNode),
				patterns: make([]*Pattern, 0),
			}
		}
		node = node.children[char]
	}
	node.isEnd = true
	node.patterns = append(node.patterns, pattern)
}

func (t *Trie) Search(path string) []*Pattern {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var results []*Pattern
	node := t.root

	for i := 0; i < len(path); i++ {
		char := path[i]

		if node.isEnd {
			results = append(results, node.patterns...)
		}

		child, ok := node.children[char]
		if !ok {
			break
		}
		node = child
	}

	if node.isEnd {
		results = append(results, node.patterns...)
	}

	return results
}

func (t *Trie) SearchExact(prefix string) []*Pattern {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root
	for i := 0; i < len(prefix); i++ {
		char := prefix[i]
		child, ok := node.children[char]
		if !ok {
			return nil
		}
		node = child
	}

	if node.isEnd {
		return node.patterns
	}
	return nil
}

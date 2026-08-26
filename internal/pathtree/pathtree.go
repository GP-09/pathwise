// Package pathtree is the format-agnostic dot-path lookup engine shared by
// pkg/json and pkg/yaml. Format packages decode into a plain `any` value and
// wrap it with NewDocument; everything else (path parsing, traversal,
// options) lives here once.
package pathtree

import "strings"

// resolveValue returns the plain value for a matched path. Containers are
// copied so callers can freely mutate results without corrupting the
// document or affecting later Get calls.
func resolveValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))

		for key, child := range value {
			out[key] = resolveValue(child)
		}

		return out

	case []any:
		out := make([]any, len(value))

		for i, child := range value {
			out[i] = resolveValue(child)
		}

		return out

	default:
		return value
	}
}

// Document wraps a decoded value and exposes the dot-path Get API.
// A Document is read-only and safe for concurrent use.
type Document struct {
	value  any
	loaded bool
}

// NewDocument wraps a decoded value into a queryable Document.
func NewDocument(value any) Document {
	return Document{value: value, loaded: true}
}

// Get returns values for the supplied dot-separated paths.
func (d Document) Get(paths ...string) map[string]any {
	return d.With().Get(paths...)
}

// With returns a Getter configured with the supplied Options.
func (d Document) With(opts ...Option) Getter {
	cfg := config{}

	for _, opt := range opts {
		opt(&cfg)
	}

	return Getter{doc: d, cfg: cfg}
}

// Getter performs Get operations using a fixed configuration produced by
// With. It is read-only and safe to reuse across multiple Get calls,
// including concurrently.
type Getter struct {
	doc Document
	cfg config
}

// Get returns values for the supplied dot-separated paths, applying whatever
// Options were configured via Document.With.
func (g Getter) Get(paths ...string) map[string]any {
	if !g.doc.loaded || len(paths) == 0 {
		return nil
	}

	w := walk{
		result: make(map[string]any, len(paths)),
		cfg:    g.cfg,
	}

	// Paths that a transform explicitly dropped must not be resurrected by
	// WithDefault, so they are tracked separately from genuine misses.
	if g.cfg.transform != nil && g.cfg.onMissing != nil {
		w.dropped = make(map[string]struct{})
	}

	w.traverse(g.doc.value, buildTrie(paths))

	if g.cfg.onMissing != nil {
		for _, path := range paths {
			if path == "" {
				continue
			}

			if _, ok := w.result[path]; ok {
				continue
			}

			if _, ok := w.dropped[path]; ok {
				continue
			}

			w.result[path] = g.cfg.onMissing(path)
		}
	}

	return w.result
}

// Option customizes the behavior of a Getter created via Document.With.
type Option func(*config)

type config struct {
	transform func(path string, value any) (any, bool)
	onMissing func(path string) any
}

// WithTransform overrides how each raw value is placed into the result map.
// Return (value, false) to drop that path from the result.
func WithTransform(fn func(path string, value any) (any, bool)) Option {
	return func(c *config) {
		c.transform = fn
	}
}

// WithDefault supplies a fallback value for paths that don't exist in the
// document, instead of omitting them from the result.
func WithDefault(fn func(path string) any) Option {
	return func(c *config) {
		c.onMissing = fn
	}
}

type trieNode struct {
	children map[string]*trieNode
	path     string
}

func buildTrie(paths []string) *trieNode {
	root := &trieNode{}

	for _, path := range paths {
		if !validPath(path) {
			continue
		}

		current := root

		start := 0

		for start < len(path) {
			end := nextSegment(path, start)

			part := path[start:end]

			if current.children == nil {
				current.children = make(map[string]*trieNode)
			}

			next, ok := current.children[part]
			if !ok {
				next = &trieNode{}

				current.children[part] = next
			}

			current = next

			if end == len(path) {
				break
			}

			start = end + 1
		}

		current.path = path
	}

	return root
}

// validPath rejects malformed dot-paths (empty, leading/trailing dot, or
// consecutive dots) so they're treated the same as a missing path rather
// than producing surprising trie matches.
func validPath(path string) bool {
	if path == "" {
		return false
	}

	if path[0] == '.' || path[len(path)-1] == '.' {
		return false
	}

	return !strings.Contains(path, "..")
}

// walk carries the per-Get mutable state through the recursive traversal.
type walk struct {
	result  map[string]any
	dropped map[string]struct{}
	cfg     config
}

// traverse is only ever invoked with a trie produced by buildTrie: the root
// call passes its non-nil return value, and every recursive call passes a
// child looked up from trie.children, which buildTrie guarantees is never
// nil. So trie is always non-nil here.
func (w *walk) traverse(value any, trie *trieNode) {
	if trie.path != "" {
		resolved := resolveValue(value)
		include := true

		if w.cfg.transform != nil {
			resolved, include = w.cfg.transform(trie.path, resolved)
		}

		if include {
			w.result[trie.path] = resolved
		} else if w.dropped != nil {
			w.dropped[trie.path] = struct{}{}
		}
	}

	if len(trie.children) == 0 {
		return
	}

	switch value := value.(type) {
	case map[string]any:
		for key, trieChild := range trie.children {
			child, ok := value[key]
			if !ok {
				continue
			}

			w.traverse(child, trieChild)
		}

	case []any:
		for key, trieChild := range trie.children {
			index, ok := arrayIndex(key)
			if !ok || index >= len(value) {
				continue
			}

			w.traverse(value[index], trieChild)
		}
	}
}

// arrayIndex parses a canonical non-negative decimal index. Forms such as
// "+1", "-0" and "01" are rejected so they resolve as missing paths rather
// than silently aliasing a valid index.
func arrayIndex(key string) (int, bool) {
	if key == "" || len(key) > 18 {
		return 0, false
	}

	if len(key) > 1 && key[0] == '0' {
		return 0, false
	}

	index := 0

	for i := 0; i < len(key); i++ {
		digit := key[i]
		if digit < '0' || digit > '9' {
			return 0, false
		}

		index = index*10 + int(digit-'0')
	}

	return index, true
}

func nextSegment(path string, start int) int {
	idx := strings.IndexByte(path[start:], '.')
	if idx == -1 {
		return len(path)
	}

	return start + idx
}

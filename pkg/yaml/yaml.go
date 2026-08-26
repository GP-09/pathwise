package yamlpath

import (
	"github.com/GP-09/pathwise/internal/pathtree"

	"gopkg.in/yaml.v3"
)

type YAML struct {
	doc pathtree.Document
}

// Option customizes the behavior of a Getter created via YAML.With.
type Option = pathtree.Option

// Getter performs Get operations using a fixed configuration produced by
// With. It is safe to reuse across multiple Get calls.
type Getter = pathtree.Getter

func Parse(data []byte) (YAML, error) {
	var value any

	if err := yaml.Unmarshal(data, &value); err != nil {
		return YAML{}, err
	}

	return YAML{
		doc: pathtree.NewDocument(value),
	}, nil
}

// Get returns values for the supplied dot-separated paths.
func (y YAML) Get(paths ...string) map[string]any {
	return y.doc.Get(paths...)
}

// GetFromBytes parses data and returns values for the supplied paths in a
// single call. Prefer Parse + Get/With when querying the same document more
// than once, since GetFromBytes re-parses data on every call.
func GetFromBytes(data []byte, paths ...string) (map[string]any, error) {
	y, err := Parse(data)
	if err != nil {
		return nil, err
	}

	return y.Get(paths...), nil
}

// With returns a Getter configured with the supplied Options, allowing
// callers to override decoding/output behavior without changing Get's
// signature.
func (y YAML) With(opts ...Option) Getter {
	return y.doc.With(opts...)
}

// WithTransform overrides how each raw value is placed into the result map.
// Return (value, false) to drop that path from the result.
func WithTransform(fn func(path string, value any) (any, bool)) Option {
	return pathtree.WithTransform(fn)
}

// WithDefault supplies a fallback value for paths that don't exist in the
// document, instead of omitting them from the result.
func WithDefault(fn func(path string) any) Option {
	return pathtree.WithDefault(fn)
}

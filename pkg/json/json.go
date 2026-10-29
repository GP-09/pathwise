package jsonpath

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/GP-09/pathwise/internal/pathtree"
)

type JSON struct {
	doc pathtree.Document
}

// Option customizes the behavior of a Getter created via JSON.With.
type Option = pathtree.Option

// Getter performs Get operations using a fixed configuration produced by
// With. It is safe to reuse across multiple Get calls.
type Getter = pathtree.Getter

func Parse(data []byte) (JSON, error) {
	return ParseWithOptions(data, ParseOptions{})
}

// ParseOptions provides opt-in validation without changing Parse's defaults.
type ParseOptions struct {
	RequireSingleValue bool
	RequireObject      bool
	ValidateFloat64    bool
}

func ParseWithOptions(data []byte, options ParseOptions) (JSON, error) {
	var value any

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&value); err != nil {
		return JSON{}, err
	}
	if options.RequireSingleValue {
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err != nil {
				return JSON{}, err
			}
			return JSON{}, fmt.Errorf("multiple JSON values")
		}
	}
	if options.RequireObject && value != nil {
		if _, ok := value.(map[string]any); !ok {
			return JSON{}, fmt.Errorf("JSON root must be an object or null")
		}
	}
	if options.ValidateFloat64 {
		if err := validateFloat64(value); err != nil {
			return JSON{}, err
		}
	}

	return JSON{
		doc: pathtree.NewDocument(value),
	}, nil
}

func validateFloat64(value any) error {
	switch value := value.(type) {
	case json.Number:
		_, err := value.Float64()
		return err
	case map[string]any:
		for _, child := range value {
			if err := validateFloat64(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateFloat64(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// FromValue is for already-decoded native metadata; it performs no JSON
// conversion. Keep the source read-only while a getter is in use.
func FromValue(value any) JSON { return JSON{doc: pathtree.NewDocument(value)} }

func WithLegacyPaths(onError ...func(error)) Option { return pathtree.WithLegacyPaths(onError...) }
func WithBorrowedValues() Option                    { return pathtree.WithBorrowedValues() }

// Get returns values for the supplied dot-separated paths.
func (j JSON) Get(paths ...string) map[string]any {
	return j.doc.Get(paths...)
}

// GetFromBytes parses data and returns values for the supplied paths in a
// single call. Prefer Parse + Get/With when querying the same document more
// than once, since GetFromBytes re-parses data on every call.
func GetFromBytes(data []byte, paths ...string) (map[string]any, error) {
	j, err := Parse(data)
	if err != nil {
		return nil, err
	}

	return j.Get(paths...), nil
}

// With returns a Getter configured with the supplied Options, allowing
// callers to override decoding/output behavior without changing Get's
// signature.
func (j JSON) With(opts ...Option) Getter {
	return j.doc.With(opts...)
}

// WithTransform overrides how each raw value is placed into the result map.
// The value may be a json.Number for numeric leaves. Return (value, false)
// to drop that path from the result.
func WithTransform(fn func(path string, value any) (any, bool)) Option {
	return pathtree.WithTransform(fn)
}

// WithDefault supplies a fallback value for paths that don't exist in the
// document, instead of omitting them from the result.
func WithDefault(fn func(path string) any) Option {
	return pathtree.WithDefault(fn)
}

package yamlpath_test

import (
	"reflect"
	"testing"

	yamlpath "pathwise/pkg/yaml"
)

func TestParse(t *testing.T) {
	t.Run("valid yaml", func(t *testing.T) {
		if _, err := yamlpath.Parse([]byte("a: 1\n")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		if _, err := yamlpath.Parse([]byte("a: [1, 2\n")); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestGetFromBytes(t *testing.T) {
	t.Run("valid yaml", func(t *testing.T) {
		data := []byte(`
hello:
  world:
    name: John
    age: 30
`)

		got, err := yamlpath.GetFromBytes(data, "hello.world.name", "hello.world.age")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := map[string]any{
			"hello.world.name": "John",
			"hello.world.age":  30,
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("invalid yaml returns error and nil result", func(t *testing.T) {
		got, err := yamlpath.GetFromBytes([]byte("a: [1, 2\n"), "a.b")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}

		if got != nil {
			t.Fatalf("expected nil result, got %#v", got)
		}
	})
}

func TestGet(t *testing.T) {
	data := []byte(`
hello:
  world:
    name: John
    age: 30
  company:
    name: Acme
tags:
  - a
  - b
  - c
nullable: null
`)

	y, err := yamlpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("nested object paths", func(t *testing.T) {
		got := y.Get("hello.world.name", "hello.world.age", "hello.company.name")

		want := map[string]any{
			"hello.world.name":   "John",
			"hello.world.age":    30,
			"hello.company.name": "Acme",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("nested path resolving to an object returns the full structure", func(t *testing.T) {
		got := y.Get("hello.world", "hello.company")

		want := map[string]any{
			"hello.world": map[string]any{
				"name": "John",
				"age":  30,
			},
			"hello.company": map[string]any{
				"name": "Acme",
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("array index path", func(t *testing.T) {
		got := y.Get("tags.0", "tags.2")

		want := map[string]any{
			"tags.0": "a",
			"tags.2": "c",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("out of range array index is omitted", func(t *testing.T) {
		got := y.Get("tags.99")

		if _, ok := got["tags.99"]; ok {
			t.Fatalf("expected tags.99 to be absent, got %#v", got)
		}
	})

	t.Run("missing path is omitted by default", func(t *testing.T) {
		got := y.Get("hello.world.name", "does.not.exist")

		if _, ok := got["does.not.exist"]; ok {
			t.Fatalf("expected does.not.exist to be absent, got %#v", got)
		}

		if got["hello.world.name"] != "John" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("null value path", func(t *testing.T) {
		got := y.Get("nullable")

		val, ok := got["nullable"]
		if !ok {
			t.Fatalf("expected nullable to be present, got %#v", got)
		}

		if val != nil {
			t.Fatalf("expected nil value, got %#v", val)
		}
	})

	t.Run("single-segment path pointing at an array returns the full slice", func(t *testing.T) {
		got := y.Get("tags")

		want := map[string]any{
			"tags": []any{"a", "b", "c"},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("array element that is itself an object is reconstructed", func(t *testing.T) {
		data := []byte(`
items:
  - id: 1
    name: a
  - id: 2
    name: b
`)

		itemsYAML, err := yamlpath.Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := itemsYAML.Get("items.0", "items.1.name")

		want := map[string]any{
			"items.0": map[string]any{
				"id":   1,
				"name": "a",
			},
			"items.1.name": "b",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("empty object and empty array are reconstructed, not omitted", func(t *testing.T) {
		data := []byte("emptyObj: {}\nemptyArr: []\n")

		emptyYAML, err := yamlpath.Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := emptyYAML.Get("emptyObj", "emptyArr")

		want := map[string]any{
			"emptyObj": map[string]any{},
			"emptyArr": []any{},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("path continuing past a scalar leaf is omitted", func(t *testing.T) {
		got := y.Get("hello.world.name.extra")

		if _, ok := got["hello.world.name.extra"]; ok {
			t.Fatalf("expected hello.world.name.extra to be absent, got %#v", got)
		}
	})

	t.Run("non-numeric key against an array is omitted", func(t *testing.T) {
		got := y.Get("tags.foo")

		if _, ok := got["tags.foo"]; ok {
			t.Fatalf("expected tags.foo to be absent, got %#v", got)
		}
	})

	t.Run("negative array index is omitted", func(t *testing.T) {
		got := y.Get("tags.-1")

		if _, ok := got["tags.-1"]; ok {
			t.Fatalf("expected tags.-1 to be absent, got %#v", got)
		}
	})

	t.Run("malformed paths are treated as missing", func(t *testing.T) {
		malformed := []string{
			".hello",            // leading dot
			"hello.world.name.", // trailing dot
			"hello..world",      // consecutive dots
		}

		got := y.Get(malformed...)

		if len(got) != 0 {
			t.Fatalf("expected malformed paths to be omitted, got %#v", got)
		}
	})

	t.Run("malformed paths receive the default via WithDefault", func(t *testing.T) {
		withFallback := yamlpath.WithDefault(func(path string) any {
			return "N/A"
		})

		got := y.With(withFallback).Get(".hello", "hello.world.name")

		want := map[string]any{
			".hello":           "N/A",
			"hello.world.name": "John",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("empty path is ignored even when WithDefault is set", func(t *testing.T) {
		withFallback := yamlpath.WithDefault(func(path string) any {
			return "N/A"
		})

		got := y.With(withFallback).Get("", "hello.world.name")

		if _, ok := got[""]; ok {
			t.Fatalf("expected empty path to be absent, got %#v", got)
		}

		if got["hello.world.name"] != "John" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("no paths returns nil", func(t *testing.T) {
		if got := y.Get(); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})

	t.Run("empty path is ignored", func(t *testing.T) {
		got := y.Get("", "hello.world.name")

		if len(got) != 1 || got["hello.world.name"] != "John" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("zero value YAML returns nil", func(t *testing.T) {
		var zero yamlpath.YAML

		if got := zero.Get("anything"); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})
}

func TestWithTransform(t *testing.T) {
	data := []byte(`
hello:
  world:
    name: John
    age: 30
    score: 1.5
`)

	y, err := yamlpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	toInt64 := yamlpath.WithTransform(func(path string, value any) (any, bool) {
		if n, ok := value.(int); ok {
			return int64(n), true
		}

		return value, true
	})

	got := y.With(toInt64).Get("hello.world.name", "hello.world.age", "hello.world.score")

	want := map[string]any{
		"hello.world.name":  "John",
		"hello.world.age":   int64(30),
		"hello.world.score": 1.5,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	t.Run("returning false omits the path", func(t *testing.T) {
		dropNames := yamlpath.WithTransform(func(path string, value any) (any, bool) {
			return value, path != "hello.world.name"
		})

		got := y.With(dropNames).Get("hello.world.name", "hello.world.age")

		if _, ok := got["hello.world.name"]; ok {
			t.Fatalf("expected hello.world.name to be omitted, got %#v", got)
		}

		if _, ok := got["hello.world.age"]; !ok {
			t.Fatalf("expected hello.world.age to be present, got %#v", got)
		}
	})
}

func TestWithDefault(t *testing.T) {
	data := []byte(`
hello:
  world:
    name: John
`)

	y, err := yamlpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	withFallback := yamlpath.WithDefault(func(path string) any {
		return "N/A"
	})

	got := y.With(withFallback).Get("hello.world.name", "hello.world.missing", "another.missing")

	want := map[string]any{
		"hello.world.name":    "John",
		"hello.world.missing": "N/A",
		"another.missing":     "N/A",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWithTransformAndDefaultCombined(t *testing.T) {
	data := []byte("age: 30\n")

	y, err := yamlpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getter := y.With(
		yamlpath.WithTransform(func(path string, value any) (any, bool) {
			if n, ok := value.(int); ok {
				return int64(n), true
			}
			return value, true
		}),
		yamlpath.WithDefault(func(path string) any {
			return int64(0)
		}),
	)

	got := getter.Get("age", "missing")

	want := map[string]any{
		"age":     int64(30),
		"missing": int64(0),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestTransformDropIsNotFilledByDefault(t *testing.T) {
	y, err := yamlpath.Parse([]byte("a: 1\nb: 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := y.With(
		yamlpath.WithTransform(func(path string, value any) (any, bool) {
			return value, path != "a"
		}),
		yamlpath.WithDefault(func(path string) any { return "DEFAULT" }),
	).Get("a", "b", "missing")

	if _, ok := got["a"]; ok {
		t.Fatalf("path dropped by transform was resurrected: %#v", got)
	}

	if got["missing"] != "DEFAULT" {
		t.Fatalf("genuinely missing path should get default: %#v", got)
	}
}

func TestNonCanonicalArrayIndexes(t *testing.T) {
	y, err := yamlpath.Parse([]byte("a:\n  - 10\n  - 20\n  - 30\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, path := range []string{"a.+1", "a.01", "a.-0", "a.-1", "a. 1", "a.1x", "a.1234567890123456789"} {
		got := y.Get(path)

		if len(got) != 0 {
			t.Fatalf("path %q should not resolve, got %#v", path, got)
		}
	}

	if got := y.Get("a.1"); got["a.1"] != 20 {
		t.Fatalf("canonical index broke: %#v", got)
	}
}

func TestGetterReuse(t *testing.T) {
	y, err := yamlpath.Parse([]byte("a: 1\nb: 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getter := y.With(yamlpath.WithTransform(func(path string, value any) (any, bool) {
		if n, ok := value.(int); ok {
			return int64(n), true
		}
		return value, true
	}))

	first := getter.Get("a")
	second := getter.Get("b")

	if first["a"] != int64(1) {
		t.Fatalf("got %#v", first)
	}

	if second["b"] != int64(2) {
		t.Fatalf("got %#v", second)
	}
}

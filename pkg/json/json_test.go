package jsonpath_test

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	jsonpath "pathwise/pkg/json"
)

func TestParse(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		if _, err := jsonpath.Parse([]byte(`{"a":1}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := jsonpath.Parse([]byte(`{invalid`)); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestGetFromBytes(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		data := []byte(`{"hello":{"world":{"name":"John","age":30}}}`)

		got, err := jsonpath.GetFromBytes(data, "hello.world.name", "hello.world.age")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := map[string]any{
			"hello.world.name": "John",
			"hello.world.age":  json.Number("30"),
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("invalid json returns error and nil result", func(t *testing.T) {
		got, err := jsonpath.GetFromBytes([]byte(`{invalid`), "a.b")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}

		if got != nil {
			t.Fatalf("expected nil result, got %#v", got)
		}
	})
}

func TestGet(t *testing.T) {
	data := []byte(`{
	"hello": {
		"world": {
			"name": "John",
			"age": 30
		},
		"company": {
			"name": "Acme"
		}
	},
	"tags": ["a", "b", "c"],
	"nullable": null,
	"encoded": "eyJoZWxsbyI6IHsiZGF0YSI6ICJIZWxsbyBXb3JsZCJ9fQ=="
}`)

	j, err := jsonpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("nested object paths", func(t *testing.T) {
		got := j.Get("hello.world.name", "hello.world.age", "hello.company.name")

		want := map[string]any{
			"hello.world.name":   "John",
			"hello.world.age":    json.Number("30"),
			"hello.company.name": "Acme",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("nested path resolving to an object returns the full structure", func(t *testing.T) {
		got := j.Get("hello.world", "hello.company")

		want := map[string]any{
			"hello.world": map[string]any{
				"name": "John",
				"age":  json.Number("30"),
			},
			"hello.company": map[string]any{
				"name": "Acme",
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("base64 encoded JSON string requires an explicit transform to decode", func(t *testing.T) {
		// Get never implicitly decodes/parses string values - it returns
		// exactly what's stored in the document. Decoding a base64+JSON
		// string into a nested structure is done via WithTransform.
		decodeBase64JSON := jsonpath.WithTransform(func(path string, value any) (any, bool) {
			s, ok := value.(string)
			if !ok {
				return value, true
			}

			raw, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return value, true
			}

			var decoded any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return value, true
			}

			return decoded, true
		})

		got := j.With(decodeBase64JSON).Get("encoded")

		want := map[string]any{
			"encoded": map[string]any{
				"hello": map[string]any{
					"data": "Hello World",
				},
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("without a transform, an encoded string is returned as-is", func(t *testing.T) {
		got := j.Get("encoded")

		want := map[string]any{
			"encoded": "eyJoZWxsbyI6IHsiZGF0YSI6ICJIZWxsbyBXb3JsZCJ9fQ==",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("array index path", func(t *testing.T) {
		got := j.Get("tags.0", "tags.2")

		want := map[string]any{
			"tags.0": "a",
			"tags.2": "c",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("out of range array index is omitted", func(t *testing.T) {
		got := j.Get("tags.99")

		if _, ok := got["tags.99"]; ok {
			t.Fatalf("expected tags.99 to be absent, got %#v", got)
		}
	})

	t.Run("missing path is omitted by default", func(t *testing.T) {
		got := j.Get("hello.world.name", "does.not.exist")

		if _, ok := got["does.not.exist"]; ok {
			t.Fatalf("expected does.not.exist to be absent, got %#v", got)
		}

		if got["hello.world.name"] != "John" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("null value path", func(t *testing.T) {
		got := j.Get("nullable")

		val, ok := got["nullable"]
		if !ok {
			t.Fatalf("expected nullable to be present, got %#v", got)
		}

		if val != nil {
			t.Fatalf("expected nil value, got %#v", val)
		}
	})

	t.Run("single-segment path pointing at an array returns the full slice", func(t *testing.T) {
		got := j.Get("tags")

		want := map[string]any{
			"tags": []any{"a", "b", "c"},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("array element that is itself an object is reconstructed", func(t *testing.T) {
		data := []byte(`{"items":[{"id":1,"name":"a"},{"id":2,"name":"b"}]}`)

		itemsJSON, err := jsonpath.Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := itemsJSON.Get("items.0", "items.1.name")

		want := map[string]any{
			"items.0": map[string]any{
				"id":   json.Number("1"),
				"name": "a",
			},
			"items.1.name": "b",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("empty object and empty array are reconstructed, not omitted", func(t *testing.T) {
		data := []byte(`{"emptyObj":{},"emptyArr":[]}`)

		emptyJSON, err := jsonpath.Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := emptyJSON.Get("emptyObj", "emptyArr")

		want := map[string]any{
			"emptyObj": map[string]any{},
			"emptyArr": []any{},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("path continuing past a scalar leaf is omitted", func(t *testing.T) {
		got := j.Get("hello.world.name.extra")

		if _, ok := got["hello.world.name.extra"]; ok {
			t.Fatalf("expected hello.world.name.extra to be absent, got %#v", got)
		}
	})

	t.Run("non-numeric key against an array is omitted", func(t *testing.T) {
		got := j.Get("tags.foo")

		if _, ok := got["tags.foo"]; ok {
			t.Fatalf("expected tags.foo to be absent, got %#v", got)
		}
	})

	t.Run("negative array index is omitted", func(t *testing.T) {
		got := j.Get("tags.-1")

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

		got := j.Get(malformed...)

		if len(got) != 0 {
			t.Fatalf("expected malformed paths to be omitted, got %#v", got)
		}
	})

	t.Run("malformed paths receive the default via WithDefault", func(t *testing.T) {
		withFallback := jsonpath.WithDefault(func(path string) any {
			return "N/A"
		})

		got := j.With(withFallback).Get(".hello", "hello.world.name")

		want := map[string]any{
			".hello":           "N/A",
			"hello.world.name": "John",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("empty path is ignored even when WithDefault is set", func(t *testing.T) {
		withFallback := jsonpath.WithDefault(func(path string) any {
			return "N/A"
		})

		got := j.With(withFallback).Get("", "hello.world.name")

		if _, ok := got[""]; ok {
			t.Fatalf("expected empty path to be absent, got %#v", got)
		}

		if got["hello.world.name"] != "John" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("no paths returns nil", func(t *testing.T) {
		if got := j.Get(); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})

	t.Run("empty path is ignored", func(t *testing.T) {
		got := j.Get("", "hello.world.name")

		if len(got) != 1 || got["hello.world.name"] != "John" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("zero value JSON returns nil", func(t *testing.T) {
		var zero jsonpath.JSON

		if got := zero.Get("anything"); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})
}

func TestWithTransform(t *testing.T) {
	data := []byte(`{"hello":{"world":{"name":"John","age":30,"score":1.5}}}`)

	j, err := jsonpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	toGoNumber := jsonpath.WithTransform(func(path string, value any) (any, bool) {
		n, ok := value.(json.Number)
		if !ok {
			return value, true
		}

		if i, err := n.Int64(); err == nil {
			return i, true
		}

		if f, err := n.Float64(); err == nil {
			return f, true
		}

		return value, true
	})

	got := j.With(toGoNumber).Get("hello.world.name", "hello.world.age", "hello.world.score")

	want := map[string]any{
		"hello.world.name":  "John",
		"hello.world.age":   int64(30),
		"hello.world.score": float64(1.5),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	t.Run("returning false omits the path", func(t *testing.T) {
		dropNames := jsonpath.WithTransform(func(path string, value any) (any, bool) {
			return value, path != "hello.world.name"
		})

		got := j.With(dropNames).Get("hello.world.name", "hello.world.age")

		if _, ok := got["hello.world.name"]; ok {
			t.Fatalf("expected hello.world.name to be omitted, got %#v", got)
		}

		if _, ok := got["hello.world.age"]; !ok {
			t.Fatalf("expected hello.world.age to be present, got %#v", got)
		}
	})
}

func TestWithDefault(t *testing.T) {
	data := []byte(`{"hello":{"world":{"name":"John"}}}`)

	j, err := jsonpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	withFallback := jsonpath.WithDefault(func(path string) any {
		return "N/A"
	})

	got := j.With(withFallback).Get("hello.world.name", "hello.world.missing", "another.missing")

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
	data := []byte(`{"age":30}`)

	j, err := jsonpath.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getter := j.With(
		jsonpath.WithTransform(func(path string, value any) (any, bool) {
			if n, ok := value.(json.Number); ok {
				if i, err := n.Int64(); err == nil {
					return i, true
				}
			}
			return value, true
		}),
		jsonpath.WithDefault(func(path string) any {
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

func TestGetterReuse(t *testing.T) {
	j, err := jsonpath.Parse([]byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getter := j.With(jsonpath.WithTransform(func(path string, value any) (any, bool) {
		if n, ok := value.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				return i, true
			}
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

func TestTransformDropIsNotFilledByDefault(t *testing.T) {
	j, err := jsonpath.Parse([]byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := j.With(
		jsonpath.WithTransform(func(path string, value any) (any, bool) {
			return value, path != "a"
		}),
		jsonpath.WithDefault(func(path string) any { return "DEFAULT" }),
	).Get("a", "b", "missing")

	if _, ok := got["a"]; ok {
		t.Fatalf("path dropped by transform was resurrected: %#v", got)
	}

	if got["missing"] != "DEFAULT" {
		t.Fatalf("genuinely missing path should get default: %#v", got)
	}
}

func TestNonCanonicalArrayIndexes(t *testing.T) {
	j, err := jsonpath.Parse([]byte(`{"a":[10,20,30]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, path := range []string{"a.+1", "a.01", "a.-0", "a.-1", "a. 1", "a.1x", "a.1234567890123456789"} {
		got := j.Get(path)

		if len(got) != 0 {
			t.Fatalf("path %q should not resolve, got %#v", path, got)
		}
	}

	if got := j.Get("a.1"); got["a.1"].(json.Number).String() != "20" {
		t.Fatalf("canonical index broke: %#v", got)
	}
}

func TestResultsAreIsolatedFromDocument(t *testing.T) {
	j, err := jsonpath.Parse([]byte(`{"a":{"b":1},"c":[1,2]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first := j.Get("a", "c")
	first["a"].(map[string]any)["b"] = "mutated"
	first["c"].([]any)[0] = "mutated"

	second := j.Get("a", "c")

	if second["a"].(map[string]any)["b"].(json.Number).String() != "1" {
		t.Fatalf("map mutation leaked into document: %#v", second)
	}

	if second["c"].([]any)[0].(json.Number).String() != "1" {
		t.Fatalf("slice mutation leaked into document: %#v", second)
	}
}

func TestConcurrentGet(t *testing.T) {
	j, err := jsonpath.Parse([]byte(`{"a":{"b":1},"c":[1,2,3]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getter := j.With()

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for n := 0; n < 100; n++ {
				if got := getter.Get("a", "c.0"); len(got) != 2 {
					t.Errorf("got %#v", got)
					return
				}
			}
		}()
	}

	wg.Wait()
}

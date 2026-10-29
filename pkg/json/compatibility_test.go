package jsonpath_test

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	jsonpath "github.com/GP-09/pathwise/pkg/json"
)

func TestLegacyOptionsLeaveDefaultsUnchanged(t *testing.T) {
	doc, err := jsonpath.Parse([]byte(`{"A.B":null,"A":{"C":"UPPER"},"items":["zero","one"],"blob":"{\"name\":\"VALUE\"}","":{"key":"empty"}}`))
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"a.b", "a.c", "items.01", "items.-0", "blob.name", ".key", "a.c.extra"}
	if got := doc.Get(paths...); len(got) != 0 {
		t.Fatalf("defaults changed: %#v", got)
	}
	got := doc.With(jsonpath.WithLegacyPaths()).Get(paths...)
	want := map[string]any{"a.b": nil, "a.c": "UPPER", "items.01": "one", "items.-0": "zero", "blob.name": "VALUE", ".key": "empty", "a.c.extra": "upper"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestLegacyWithTransformAndDefault(t *testing.T) {
	doc, err := jsonpath.Parse([]byte(`{"Value":3,"Drop":1}`))
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	getter := doc.With(jsonpath.WithLegacyPaths(), jsonpath.WithTransform(func(path string, value any) (any, bool) {
		calls[path]++
		if path == "drop" {
			return nil, false
		}
		number, ok := value.(json.Number)
		if !ok {
			t.Fatalf("transform received %T", value)
		}
		n, err := number.Float64()
		if err != nil {
			t.Fatal(err)
		}
		return n, true
	}), jsonpath.WithDefault(func(string) any { return "default" }))
	got := getter.Get("value", "value", "drop", "missing")
	want := map[string]any{"value": float64(3), "missing": "default"}
	if !reflect.DeepEqual(got, want) || calls["value"] != 1 || calls["drop"] != 1 {
		t.Fatalf("got %#v calls %#v", got, calls)
	}
}

func TestParseValidationIsOptIn(t *testing.T) {
	for _, raw := range []string{`{"ignored":1e309}`, `{"a":1} {"b":2}`, `[]`, `"scalar"`} {
		if _, err := jsonpath.Parse([]byte(raw)); err != nil {
			t.Fatalf("default Parse changed for %s: %v", raw, err)
		}
		if _, err := jsonpath.ParseWithOptions([]byte(raw), jsonpath.ParseOptions{
			RequireSingleValue: true, RequireObject: true, ValidateFloat64: true,
		}); err == nil {
			t.Fatalf("validation accepted %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"count":3,"items":[1,2]}`} {
		if _, err := jsonpath.ParseWithOptions([]byte(raw), jsonpath.ParseOptions{
			RequireSingleValue: true, RequireObject: true, ValidateFloat64: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	doc, err := jsonpath.Parse([]byte(`{"id":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Get("id")["id"]; got != json.Number("9007199254740993") {
		t.Fatalf("default precision changed: %#v", got)
	}
}

func TestNativeContainersAndConcurrentLegacyGet(t *testing.T) {
	source := map[string]any{"Count": int64(9007199254740993), "Object": map[string]any{"Name": "Mixed"}}
	doc := jsonpath.FromValue(source)
	copy := doc.With(jsonpath.WithLegacyPaths()).Get("object")["object"].(map[string]any)
	copy["Name"] = "changed"
	if source["Object"].(map[string]any)["Name"] != "Mixed" {
		t.Fatal("default no longer copies")
	}
	getter := doc.With(jsonpath.WithLegacyPaths(), jsonpath.WithBorrowedValues())
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if got := getter.Get("count", "object.name"); got["count"] != int64(9007199254740993) || got["object.name"] != "Mixed" {
					t.Errorf("got %#v", got)
					return
				}
			}
		}()
	}
	wg.Wait()
	borrowed := getter.Get("object")["object"].(map[string]any)
	borrowed["Name"] = "alias"
	if source["Object"].(map[string]any)["Name"] != "alias" {
		t.Fatal("borrow option lost identity")
	}
}

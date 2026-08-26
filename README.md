# pathwise

Dot-path value lookups for JSON and YAML documents, without unmarshaling into
structs. Ask for one or more `"a.b.c"` paths, get back a
`map[string]any` keyed by exactly the paths you asked for.

```go
j, _ := jsonpath.Parse(data)

values := j.Get("hello.world.name", "hello.world.age")
// map[string]any{
//   "hello.world.name": "John",
//   "hello.world.age":  json.Number("30"),
// }
```

## Packages

| Package | Import | Format |
|---|---|---|
| `pkg/json` | `jsonpath "pathwise/pkg/json"` | JSON |
| `pkg/yaml` | `yamlpath "pathwise/pkg/yaml"` | YAML |
| `internal/pathtree` | not importable outside this module | shared engine |

`pkg/json` and `pkg/yaml` expose the **same API shape** — only the decoding
step differs (`encoding/json` vs `gopkg.in/yaml.v3`). Everything else (path
parsing, tree traversal, options) is implemented **once** in
`internal/pathtree` and reused by both.

## Architecture

```
                +-----------------------+
  bytes ------->|  jsonpath.Parse       |----+
                |  yamlpath.Parse       |    |  decode into
                +-----------------------+    |  map[string]any / []any / scalar
                                              v
                                   +----------------------+
                                   | pathtree.NewDocument |
                                   |  -> builds a node    |
                                   |     tree             |
                                   +----------------------+
                                              |
                              j.Get(...) / j.With(opts...).Get(...)
                                              |
                                   +----------------------+
                                   |  pathtree.Document/  |
                                   |  pathtree.Getter     |
                                   |  - buildTrie(paths)  |
                                   |  - traverse(...)     |
                                   +----------------------+
                                              |
                                    map[string]any result
```

- **`Parse`** decodes raw bytes into a plain `any` (JSON uses
  `json.Number` for numbers so precision isn't lost; YAML gives native Go
  types like `int`/`float64`).
- **`pathtree.NewDocument`** wraps that into an internal `node` tree once, so
  it can be queried many times cheaply.
- **`Get`** builds a small trie out of the requested dot-paths, then walks
  the document tree once, only visiting branches that some requested path
  actually needs.

## API

### Parse once, query many times

```go
j, err := jsonpath.Parse(data)
if err != nil { ... }

values := j.Get("hello.world.name", "hello.world.age")
```

### One-shot convenience

```go
values, err := jsonpath.GetFromBytes(data, "hello.world.name", "hello.world.age")
```

`GetFromBytes` re-parses `data` every call — prefer `Parse` + `Get` if you're
querying the same document more than once.

### Overriding behavior with `With`

`Get` has a fixed signature (`paths ...string`), so customization is done via
a separate, chainable entry point instead of a growing list of parameters:

```go
values := j.With(
    // Convert json.Number into a concrete int64/float64.
    jsonpath.WithTransform(func(path string, value any) (any, bool) {
        if n, ok := value.(json.Number); ok {
            if i, err := n.Int64(); err == nil {
                return i, true
            }
            if f, err := n.Float64(); err == nil {
                return f, true
            }
        }
        return value, true // keep as-is
    }),
    // Supply a fallback for paths that don't exist in the document.
    jsonpath.WithDefault(func(path string) any {
        return nil
    }),
).Get("hello.world.name", "hello.world.age", "hello.world.missing")
```

- `WithTransform(fn)` — called once per requested path with the raw decoded
  value. Return `(value, false)` to drop that path from the result entirely
  (e.g. to filter out unwanted fields).
- `WithDefault(fn)` — called once per path that wasn't found in the document,
  so it appears in the result with a fallback instead of being silently
  omitted.
- `j.With(opts...)` returns a `Getter`, which is safe to build once and reuse
  across many `Get` calls (e.g. build it once with your transform, then call
  `.Get(...)` per request).

### Decoding embedded/encoded strings (e.g. base64+JSON)

`Get` never implicitly parses string contents — a base64 string stays a
string unless you decode it yourself via `WithTransform`:

```go
decode := jsonpath.WithTransform(func(path string, value any) (any, bool) {
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

j.With(decode).Get("encoded") // -> decoded, nested map[string]any
```

## Path semantics & edge cases

| Input | Behavior |
|---|---|
| `"hello.world.name"` | returns the scalar leaf value |
| `"hello.world"` (points at an object) | returns the full `map[string]any` subtree |
| `"tags"` (points at an array) | returns the full `[]any` subtree |
| `"tags.0"`, `"tags.2"` | array indexing by position |
| `"tags.99"` (out of range) | omitted from the result |
| `"tags.-1"` (negative index) | omitted from the result |
| `"tags.foo"` (non-numeric key on an array) | omitted from the result |
| `"hello.world.name.extra"` (path continues past a scalar) | omitted from the result |
| a path with no matching key | omitted, unless `WithDefault` is set |
| `""`, `".foo"`, `"foo."`, `"foo..bar"` (malformed) | treated as missing — omitted, or defaulted via `WithDefault` |
| `null` in the document | present in the result with value `nil` |
| `{}` / `[]` in the document | reconstructed as empty `map[string]any{}` / `[]any{}`, not `nil` |
| zero-value `JSON{}` / no paths passed | `Get` returns `nil` |

## Adding a new format

To support another format (e.g. TOML), add `pkg/toml/toml.go` that:
1. Decodes bytes into `map[string]any` / `[]any` / scalars.
2. Wraps the result with `pathtree.NewDocument(value)`.
3. Re-exports `Parse`, `Get`, `GetFromBytes`, `With`, `WithTransform`,
   `WithDefault`, and type aliases for `Option`/`Getter` — mirroring
   `pkg/json/json.go`.

No changes to `internal/pathtree` are needed; it's already format-agnostic.

## Running tests

```sh
go test ./pkg/... ./internal/... -race -cover
```

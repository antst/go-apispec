# go-apispec: Generate OpenAPI from Go code

[![CI](https://github.com/antst/go-apispec/actions/workflows/ci.yml/badge.svg)](https://github.com/antst/go-apispec/actions/workflows/ci.yml)
[![Release](https://github.com/antst/go-apispec/actions/workflows/release.yml/badge.svg)](https://github.com/antst/go-apispec/actions/workflows/release.yml)
![Coverage](https://img.shields.io/badge/coverage-96.3%25-brightgreen.svg)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/antst/go-apispec/blob/main/LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/antst/go-apispec.svg)](https://pkg.go.dev/github.com/antst/go-apispec)

**go-apispec** analyzes your Go source code and generates an OpenAPI 3.1 spec (YAML or JSON). Point it at your module — it detects the framework, follows the call graph from routes to handlers, and infers request/response types from real code.

## Quick Start

```bash
# Install
go install github.com/antst/go-apispec/cmd/apispec@latest

# Generate (auto-detects framework)
apispec --dir ./your-project --output openapi.yaml
```

That's it. The tool detects your framework, finds all routes, resolves handler types, and writes the spec.

## Features

### Framework Support

| Framework | Routes | Params | Request Body | Responses | Mounting/Groups |
|-----------|--------|--------|-------------|-----------|-----------------|
| **Chi** | Full | `chi.URLParam`, `r.FormValue`, `r.FormFile`, render pkg | `json.Decode`, `render.DecodeJSON` | `json.Encode`, `render.JSON`, `w.Write` | `Mount`, `Group` |
| **Gin** | Full | `c.Param`, `c.Query` | `ShouldBindJSON`, `BindJSON` | `c.JSON`, `c.String`, `c.Data` | `Group` |
| **Echo** | Full | `c.Param`, `c.QueryParam`, `r.FormValue`, `r.FormFile` | `c.Bind` | `c.JSON`, `c.String`, `c.Blob` | `Group` |
| **Fiber** | Full | `c.Params`, `c.Query`, `c.FormValue`, `c.FormFile` | `c.BodyParser` | `c.JSON`, `c.Status().JSON` | `Mount`, `Group` |
| **Gorilla Mux** | Full | Path template `{id}` (regex constraints `{id:[0-9]+}` stripped), `.Queries("q", "{q}")` | `json.Decode` | `json.Encode`, `w.Write` | `PathPrefix`, `Subrouter` |
| **net/http** | Basic | Path template, `r.FormValue`, `r.FormFile` | `json.Decode` | `json.Encode`, `w.Write`, `http.Error` | Nested `ServeMux` |

Go 1.22+ `ServeMux` method-prefix patterns are supported — `mux.HandleFunc("GET /health/live", h)` produces `get: /health/live`, not a literal `/GET /health/live` key. Path templates are normalized per router: gorilla/mux regex constraints (`{id:[0-9]+}` → `{id}`), Go 1.22 trailing wildcards (`{path...}` → `{path}`), and the `{$}` end-of-path anchor (dropped) all reduce to clean OpenAPI placeholders, with the path parameters inferred.

Projects using **multiple frameworks** simultaneously are fully supported — all routes from all detected frameworks appear in the spec.

All frameworks also detect `fmt.Fprintf`, `io.Copy`, and `io.WriteString` as response writes.

### Analysis Capabilities

**Response Detection**
- Content-Type inference from `w.Header().Set("Content-Type", "image/png")`
- Dynamic content-type fallback: `w.Header().Set("Content-Type", doc.MimeType)` → `application/octet-stream` (variable MIME types don't leak Go field paths into the spec)
- `WriteHeader(201)` + `json.Encode(user)` merged into a single 201 response with schema
- Error helper functions: `writeJSONError(w, http.StatusBadRequest, "msg")` → 400 response with ErrorResponse schema, traced through function parameters via ParamArgMap
- Multiple calls to same helper: `writeJSONError(w, 400, ...)` + `writeJSONError(w, 404, ...)` → both status codes captured with correct schemas
- Status code variable resolution: `status := http.StatusCreated; w.WriteHeader(status)` → 201
- Cross-function status codes: `w.WriteHeader(getStatus())` where `getStatus()` returns a constant
- Multiple response types for the same status code → `oneOf` schema
- `[]byte` responses → `type: string, format: binary`. A free-function write (`io.Copy`, `io.WriteString`, `fmt.Fprintf`) only counts as a response when its **destination is the HTTP response writer** — an `io.Copy(file, src)` or `io.Copy(io.Discard, r)` reachable deep in the call graph is not misread as the endpoint's body
- Conditional-status fan-out: a status-carrying error variable reassigned across branches (`if … { err = NewError(msg, http.StatusBadRequest) } else { err = NewError(msg, http.StatusNotFound) }`) then passed to one `RespondWithError(w, err)` → one response per status. The set is filtered to the assignments that actually **reach that call site**, so a later/overwritten/early-return-only reassignment doesn't leak an unrelated status
- Bodyless status codes (1xx, 204, 304) never get body schemas (per RFC 7231)
- Implicit 200 for handlers that write a body without explicit `WriteHeader`

**Type Resolution**
- Generic struct instantiation: `APIResponse[User]` → schema with `Data: $ref User`, bound per call site — a handler returning `APIResponse[User]{...}` and another returning `APIResponse[Product]{...}` each get their concrete payload, not an unbound `T`
- Interface resolution: handlers registered via interface → concrete implementation schemas
- `interface{}` parameter resolution: `respondJSON(w, 201, user)` where `data` is `interface{}` → resolves to concrete `User` type from the caller's argument
- Conditional HTTP methods via CFG: `switch r.Method { case "GET": ... case "POST": ... }` → separate operations
- Route path variables: `path := "/users"; r.GET(path, h)` → resolves variable to literal path
- Decode receiver tracing: `json.NewDecoder(file).Decode(&cfg)` not misclassified as request body. The data source is validated for arg-sourced decodes too — `json.Unmarshal(dbColumn, &v)` reachable deep in a handler's call graph is only treated as the request body when its source traces back to `r.Body`
- io.Copy source tracing: `io.Copy(w, strings.NewReader(...))` → `type: string`; `io.Copy(w, file)` → `format: binary`

**Documentation Extraction**
- Go doc comments on handler functions → OpenAPI `summary` and `description`
- First sentence → `summary`, full comment → `description` (single-sentence comments don't duplicate)
- No annotations needed — existing Go doc comments just work
- Config overrides take precedence over doc comments

**Schema Inference**
- Required fields from `json:",omitempty"` absence and `binding:"required"` tags
- Validator `dive` tag: `validate:"dive,email"` on `[]string` → items schema has `format: email`
- Validator bound tags → schema constraints: `gte`/`lte` (and `min`/`max`) on a numeric field → `minimum`/`maximum`; the same tags on a string field → `minLength`/`maxLength` (go-playground/validator treats them as length); `oneof` → `enum`; `len`/`minlen`/`maxlen` → exact/min/max length
- Embedded struct promotion: anonymously embedded structs (by value, by pointer, and transitively) have their fields flattened onto the outer schema, matching Go's field promotion and flat JSON marshaling
- `Mux.Vars()` map index expressions → path parameter names
- Type mappings for `time.Time`, `uuid.UUID`, and custom types
- Container value types: `map[string]Struct` → `additionalProperties: $ref`, `map[string][]Struct` → `additionalProperties` array of `$ref`; nested slices `[][]T` → nested array schemas (no mangled component names)
- Converter-typed params: `idStr := c.Param("id"); strconv.Atoi(idStr)` → `integer`; `strconv.ParseBool` → `boolean`; `strconv.ParseFloat` → `number`; `uuid.Parse` → `string/uuid`. Works for both inline (`strconv.Atoi(r.FormValue("x"))`) and var-bound (`if v := r.FormValue("x"); v != "" { strconv.ParseBool(v) }`) idioms, including shadowed variables in separate `if`-init scopes
- `r.FormFile("upload")` → form parameter with `string`/`binary` schema
- JSON-DTO field flow inference: when a decoded body's field is later passed to a converter (`uuid.Parse(body.SourceID)`, including pointer-deref `uuid.Parse(*body.TagsetID)`), apispec back-propagates the converter's schema (e.g. `format: uuid`) onto the struct field
- Explicit per-field overrides via the `apispec:"format=uuid,type=string"` struct tag — covers fields the flow analysis can't reach (e.g. UUIDs that are read but never parsed in the handler, or `format: date-time` / `format: email` hints)
- `requestBody.required: true` is emitted automatically when a handler reads the body via `json.Decode`, `json.Unmarshal`, `c.Bind`, `c.BodyParser`, etc.
- Struct-level validation via a blank-marker field — `_ struct{}` with tag `apispec:"minProperties=1,anyOf=displayName|storageBucketId|temporaryLocation"` — emits OpenAPI `minProperties` and per-field `anyOf` blocks. The marker is invisible in JSON; tag values use `,` for key=value pairs and `|` to separate field names inside `anyOf`. Useful for PATCH endpoints that require at least one field
- Security scheme detection — handlers that read `r.Header.Get("Authorization")` and trim a scheme prefix (`strings.TrimPrefix(_, "Bearer ")` etc.) get an OpenAPI `components.securitySchemes` entry and a per-operation `security:` reference. Detection follows the call graph transitively, so a shared `ValidateBearerToken(r, ...)` helper is recognised the same way as an inline pattern. Recognised prefixes: `Bearer` → `http`/`bearer`, `Basic` → `http`/`basic`, raw header (no prefix) → `apiKey` in header. The redundant `Authorization` header parameter is suppressed automatically. Override or extend by declaring `securitySchemes:` in `apispec.yaml` — config entries win over detection for the same scheme name

**Output Quality**
- Deterministic YAML/JSON — sorted map keys, identical output across runs, safe for CI diffing
- Short names by default — `DocumentHandler.GetContent` instead of `github.com/org/.../http.Deps.DocumentHandler.GetContent`
- Config merging — `--config` extends auto-detected framework defaults instead of replacing them

### Call Graph Visualization

Interactive Cytoscape.js diagrams with:
- Hierarchical tree layout with zoom, pan, and click-to-highlight
- CFG branch coloring: green (if-then), red dashed (if-else), purple (switch-case)
- Branch labels showing case values (e.g., "GET", "POST")
- Paginated mode for large graphs (1000+ edges)
- PNG/SVG export

```bash
apispec --dir ./my-project --output openapi.yaml --diagram diagram.html
```

## Usage

```bash
# Basic generation
apispec --dir ./my-project --output openapi.yaml

# With custom config
apispec --dir ./my-project --config apispec.yaml --output openapi.yaml

# Legacy naming (fully-qualified operationIds and schema names)
apispec --dir ./my-project --output openapi.yaml --short-names=false

# With call graph diagram
apispec --dir ./my-project --output openapi.yaml --diagram diagram.html

# Skip CGO packages
apispec --dir ./my-project --output openapi.yaml --skip-cgo

# Tune limits for large codebases
apispec --dir ./my-project --output openapi.yaml --max-nodes 100000 --max-recursion-depth 15

# Performance profiling
apispec --dir ./my-project --output openapi.yaml --cpu-profile --mem-profile
```

### Key Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `openapi.json` | Output file (`.yaml`/`.json`) |
| `--dir` | `-d` | `.` | Project directory |
| `--config` | `-c` | — | Custom YAML config |
| `--short-names` | — | `true` | Strip module paths from names |
| `--diagram` | `-g` | — | Save call graph as HTML |
| `--title` | `-t` | `Generated API` | API title |
| `--api-version` | `-v` | `1.0.0` | API version |
| `--skip-cgo` | — | `true` | Skip CGO packages |
| `--max-nodes` | `-mn` | `50000` | Max call graph nodes |
| `--max-recursion-depth` | `-mrd` | `10` | Max recursion depth |
| `--verbose` | `-vb` | `false` | Verbose output |

Full flag list: `apispec --help`

## Programmatic Usage

```go
import (
    "os"
    "github.com/antst/go-apispec/generator"
    "github.com/antst/go-apispec/spec"
    "gopkg.in/yaml.v3"
)

func main() {
    cfg := spec.DefaultChiConfig() // or DefaultGinConfig, DefaultEchoConfig, etc.
    gen := generator.NewGenerator(cfg)
    openapi, err := gen.GenerateFromDirectory("./your-project")
    if err != nil { panic(err) }
    data, _ := yaml.Marshal(openapi)
    os.WriteFile("openapi.yaml", data, 0644)
}
```

## Configuration

Auto-detection works for most projects. For custom behavior, create `apispec.yaml`:

```yaml
info:
  title: My API
  version: 2.0.0

shortNames: true  # false for legacy fully-qualified names

framework:
  routePatterns:
    - callRegex: ^(?i)(GET|POST|PUT|DELETE|PATCH)$
      recvTypeRegex: ^github\.com/gin-gonic/gin\.\*(Engine|RouterGroup)$
      handlerArgIndex: 1
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true

typeMapping:
  - goType: time.Time
    openapiType: { type: string, format: date-time }
  - goType: uuid.UUID
    openapiType: { type: string, format: uuid }

externalTypes:
  - name: github.com/gin-gonic/gin.H
    openapiType: { type: object, additionalProperties: true }
```

Full configuration examples for each framework: see the `Default*Config()` functions in `internal/spec/config.go` for the built-in patterns.

## How It Works

```
Go source → Package loading & type-checking → Framework detection
  → AST traversal → Call graph + CFG construction → Pattern matching
  → OpenAPI mapping → YAML/JSON output
```

1. Loads and type-checks all Go packages in the module
2. Detects web frameworks from imports (supports multiple frameworks simultaneously)
3. Builds a call graph from router registrations to handlers
4. Builds a control-flow graph (CFG) via `golang.org/x/tools/go/cfg` for branch analysis
5. Matches route, request, response, and parameter patterns against the call graph
6. Resolves conditional methods, generic types, and interface implementations using CFG and type analysis
7. Maps Go types to OpenAPI schemas (structs, enums, aliases, generics, validators)
8. Serializes with sorted keys for deterministic output

## Known Limitations

These are inherent to static analysis — analyzing code without executing it:

| Limitation | Example | What Happens |
|-----------|---------|-------------|
| **Reflection-based routing** | Routes registered via `reflect.Value.Call` | Not visible in static analysis |
| **Computed paths** | `r.GET("/api/" + version, handler)` | String concatenation not evaluated; only literal and variable-assigned paths resolved |
| **Complex cross-function values** | `func compute() int { return a + b }; WriteHeader(compute())` | Only functions with a single constant return are resolved; computed values are not traced |

## Interactive Diagram Server

```bash
# Build and start
go build -o apidiag ./cmd/apidiag
./apidiag --dir ./my-project --port 8080
# Open http://localhost:8080
```

Provides a web UI for exploring call graphs with filtering, pagination, and export. See [cmd/apidiag/README.md](cmd/apidiag/README.md).

## Development

```bash
# Build
make build

# Test
make test

# Lint
make lint

# Coverage
make coverage

# Update golden files after intentional output changes
# (generates to temp, shows diff, requires confirmation)
go test ./internal/engine/ -run TestUpdateGolden -v
```

### Project Structure

```
go-apispec/
├── cmd/apispec/          CLI entry point
├── cmd/apidiag/          Interactive diagram server
├── generator/            High-level generator API
├── spec/                 Public types (re-exports from internal/spec)
├── internal/
│   ├── core/             Framework detection
│   ├── engine/           Generation engine
│   ├── metadata/         AST analysis and metadata extraction
│   └── spec/             OpenAPI mapping, patterns, schemas
├── pkg/patterns/         Gitignore-style pattern matching
└── testdata/             Framework fixtures + golden files
```

### Golden File Tests

Every `testdata/` directory has `expected_openapi.json` (short names) and `expected_openapi_legacy.json` (fully-qualified). Golden files use relative paths only — no machine-specific absolute paths.

**One code path**: `generateGoldenSpec()` is the single function used by both comparison and generation. Tests never overwrite golden files.

**Determinism is enforced, not assumed**: `TestGolden_Deterministic` regenerates every fixture several times in-process (Go randomizes map iteration each run, exercising different orderings) and asserts byte-identical output across runs — for both the default and legacy snapshots. This is what makes `make openapi` + `git diff --exit-code` a reliable CI staleness gate.

```bash
# Compare (runs in CI — fails on mismatch):
go test ./internal/engine/ -run TestGolden -v

# Verify output is deterministic across runs:
go test ./internal/engine/ -run TestGolden_Deterministic

# Update after intentional changes (explicit, never automatic):
go test ./internal/engine/ -run TestUpdateGolden -v
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short:

1. Fork, branch, make changes
2. Add tests (`make test`)
3. Run lint (`make lint`)
4. Open a PR

## License

Apache License 2.0 — see [LICENSE](LICENSE).

Originally forked from [apispec](https://github.com/ehabterra/apispec) by Ehab Terra.

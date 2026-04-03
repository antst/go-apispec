# go-apispec: Generate OpenAPI from Go code

[![CI](https://github.com/antst/go-apispec/actions/workflows/ci.yml/badge.svg)](https://github.com/antst/go-apispec/actions/workflows/ci.yml)
[![Release](https://github.com/antst/go-apispec/actions/workflows/release.yml/badge.svg)](https://github.com/antst/go-apispec/actions/workflows/release.yml)
![Coverage](https://img.shields.io/badge/coverage-84.5%25-yellow.svg)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/antst/go-apispec/blob/main/LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/antst/go-apispec.svg)](https://pkg.go.dev/github.com/antst/go-apispec)

> This project originated as a fork of [apispec](https://github.com/ehabterra/apispec) by Ehab Terra. It has since been substantially rewritten — the core analysis pipeline, pattern matching, type resolution, schema generation, and test infrastructure have been reworked. The original codebase provided the foundational architecture; the current implementation diverges significantly.

**go-apispec** analyzes your Go source code and generates an OpenAPI 3.1 spec (YAML or JSON). Point it at your module — it detects the framework, follows the call graph from routes to handlers, and infers request/response types from real code.

## Quick Start

```bash
# Install
go install github.com/antst/go-apispec/cmd/apispec@latest

# Generate (auto-detects framework)
apispec --dir ./your-project --output openapi.yaml
```

That's it. The tool detects your framework (Gin, Echo, Chi, Fiber, Gorilla Mux, net/http), finds all routes, resolves handler types, and writes the spec.

## What's Different from the Original

| Area | Change |
|------|--------|
| **Deterministic output** | Sorted map keys in YAML/JSON — identical output across runs, safe for CI diffing |
| **Short names** | operationIds and schema names strip Go module paths by default (`DocumentHandler.GetContent` not `github.com/org/.../http.Deps.DocumentHandler.GetContent`) |
| **`[]byte` binary mapping** | `w.Write(data)` where data is `[]byte` produces `type: string, format: binary` |
| **Multiple response types** | Same status code with different types produces `oneOf` schema |
| **Required field inference** | Fields without `json:",omitempty"` are automatically `required` |
| **Binding tag support** | Gin's `binding:"required"` parsed for required field detection |
| **Implicit 200 inference** | Handlers that write a body without `WriteHeader` get `200`, not `default` |
| **stdlib write detection** | `fmt.Fprintf`, `io.Copy`, `io.WriteString` detected as response writes |
| **Bodyless status codes** | 1xx, 204, 304 never get body schemas (per RFC 7231) |
| **WriteHeader+Encode merge** | `w.WriteHeader(201); json.Encode(user)` correctly produces 201 response with User schema |
| **Config merging** | `--config` merges with auto-detected framework defaults instead of replacing them |
| **Content-Type inference** | `w.Header().Set("Content-Type", "image/png")` → response uses `image/png` instead of default `application/json` |
| **Status code variables** | `status := http.StatusCreated; w.WriteHeader(status)` → correctly resolves to 201 |
| **Mux.Vars() params** | `vars["id"]` map index expressions detected as path parameter names |
| **Validator `dive` tag** | `validate:"dive,email"` on `[]string` → items schema has `format: email` |
| **Decode receiver tracing** | `json.NewDecoder(file).Decode(&cfg)` not misclassified as request body |
| **io.Copy source tracing** | `io.Copy(w, strings.NewReader(...))` → `type: string`; `io.Copy(w, file)` → `format: binary` |
| **Route path variables** | `path := "/users"; r.GET(path, h)` → resolves variable to literal path |
| **Conditional HTTP methods** | `switch r.Method { case "GET": ... case "POST": ... }` produces separate operations per method via CFG analysis |
| **Generic struct instantiation** | `APIResponse[User]` generates schema with `Data: $ref User, Error: string` |
| **Interface resolution** | Handlers registered via interface are resolved to concrete implementations |
| **Control-flow graph** | `golang.org/x/tools/go/cfg` provides branch context for conditional analysis |
| **Cross-function status codes** | `w.WriteHeader(getStatus())` where `getStatus()` returns a constant → correctly resolved |
| **Architecture cleanup** | Shared BasePattern/MatchNode/resolveTypeOrigin, unified visitor, decomposed processArguments |
| **Bug fixes** | Mux spurious params, Fiber double mount paths, index expression types, `minimum: 0` serialization |
| **Test coverage** | `internal/spec`: 46% → 81%. Golden-file regression tests for all frameworks. |

Use `--short-names=false` to get the original fully-qualified naming if your tooling depends on it.

## Supported Frameworks

| Framework | Routes | Params | Request Body | Responses | Mounting/Groups |
|-----------|--------|--------|-------------|-----------|-----------------|
| **Chi** | Full | `chi.URLParam`, render pkg | `json.Decode`, `render.DecodeJSON` | `json.Encode`, `render.JSON`, `w.Write` | `Mount`, `Group` |
| **Gin** | Full | `c.Param`, `c.Query` | `ShouldBindJSON`, `BindJSON` | `c.JSON`, `c.String`, `c.Data` | `Group` |
| **Echo** | Full | `c.Param`, `c.QueryParam` | `c.Bind` | `c.JSON`, `c.String`, `c.Blob` | `Group` |
| **Fiber** | Full | `c.Params`, `c.Query` | `c.BodyParser` | `c.JSON`, `c.Status().JSON` | `Mount`, `Group` |
| **Gorilla Mux** | Full | Path template `{id}` | `json.Decode` | `json.Encode`, `w.Write` | `PathPrefix`, `Subrouter` |
| **net/http** | Basic | Path template | `json.Decode` | `json.Encode`, `w.Write`, `http.Error` | — |

All frameworks also detect `fmt.Fprintf`, `io.Copy`, and `io.WriteString` as response writes.

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

Full configuration examples for each framework: [docs/CONFIGURATION.md](docs/) (see the `Default*Config()` functions in `internal/spec/config.go` for the built-in patterns).

## How It Works

```
Go source → Package loading & type-checking → Framework detection
  → AST traversal → Call graph + CFG construction → Pattern matching
  → OpenAPI mapping → YAML/JSON output
```

1. Loads and type-checks all Go packages in the module
2. Detects the web framework from `go.mod` dependencies
3. Builds a call graph from router registrations to handlers
4. Builds a control-flow graph (CFG) via `golang.org/x/tools/go/cfg` for branch analysis
5. Matches route, request, response, and parameter patterns against the call graph
6. Resolves conditional methods, generic types, and interface implementations using CFG and type analysis
7. Maps Go types to OpenAPI schemas (structs, enums, aliases, generics, validators)
8. Serializes with sorted keys for deterministic output

## Known Limitations

### Static Analysis Limitations

These are inherent to the approach of analyzing code without executing it:

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

```bash
# Compare (runs in CI — fails on mismatch):
go test ./internal/engine/ -run TestGolden -v

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

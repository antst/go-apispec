# Interface Resolution in go-apispec

## Overview

go-apispec resolves interface types to their concrete implementations at two levels:

1. **Embedded interface resolution** — when a struct embeds an interface and initializes it with a concrete type during construction (dependency injection pattern)
2. **Route handler resolution** — when a route handler is registered via an interface method, the tool finds the concrete implementation's response types

Both are automatic — no manual configuration needed.

## Embedded Interface Resolution

### The Pattern

```go
type Handlers struct {
    AuthorHandler  // embedded interface
    BookHandler    // embedded interface
}

func New(s *services.Services) *Handlers {
    return &Handlers{
        AuthorHandler: &authorHandler{s.Author},
        BookHandler:   &bookHandler{s.Book},
    }
}
```

The metadata layer detects that `AuthorHandler` is initialized with `*authorHandler` by analyzing the struct literal in `New()`. The tracker tree maps interface methods to their concrete implementations automatically.

### How It Works

During AST traversal, `registerEmbeddedInterfaceResolution()` in `metadata.go` detects `KeyValueExpr` assignments where the key is an interface field and the value is a concrete type. The mapping is stored in the tracker's interface resolution registry.

During route extraction, when a method call like `h.GetAuthors()` is encountered on the `Handlers` struct, `ResolveInterfaceFromMetadata()` resolves `AuthorHandler` → `*authorHandler`, allowing the tool to trace into the concrete method's body for response types.

## Route Handler Interface Resolution

### The Pattern

```go
type ContentServer interface {
    Serve(w http.ResponseWriter, r *http.Request)
}

type FileServer struct{}

func (fs *FileServer) Serve(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(FileMetadata{Name: "doc.pdf", Size: 1024})
}

// Route registered via interface:
var server ContentServer = &FileServer{}
r.Get("/files/serve", server.Serve)
```

### How It Works

1. The route extractor detects that the handler (`server.Serve`) is a method on an interface type
2. `isInterfaceHandler()` checks the metadata to confirm the type is an `interface` (not a struct)
3. `resolveInterfaceHandler()` searches the call graph for concrete methods with the same name and package
4. Response patterns are matched against the concrete method's call graph edges using a `callGraphEdgeNode` wrapper
5. The concrete `FileServer.Serve` method's `Encode(FileMetadata{...})` call is detected and produces the correct response schema

### Result

```yaml
/files/serve:
  get:
    operationId: ContentServer.Serve
    responses:
      "200":
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/FileMetadata'
```

## API Reference

### TrackerTree Methods

- `RegisterInterfaceResolution(interfaceType, structType, pkg, concreteType string)` — register a mapping from interface to concrete type in a struct context
- `ResolveInterface(interfaceType, structType, pkg string) string` — resolve interface to concrete type
- `ResolveInterfaceFromMetadata(recvType, funcType, pkg string) string` — resolve using metadata's `ImplementedBy` tracking
- `GetInterfaceResolutions() map[interfaceKey]string` — return all registered resolutions
- `SyncInterfaceResolutionsFromMetadata()` — populate resolutions from metadata analysis

### Metadata Analysis

- `analyzeInterfaceImplementations()` — populates `Type.Implements` and `Type.ImplementedBy` fields for all struct/interface pairs
- `registerEmbeddedInterfaceResolution()` — detects concrete types in struct literal initializers

## Limitations

- Route handler resolution requires the interface and implementation to be in the same package (or the call graph to contain both)
- If multiple concrete types implement the same interface, the first match in the call graph is used
- Interface resolution for handler dispatch works for method-style registration (`server.Serve`), not for wrapped function patterns

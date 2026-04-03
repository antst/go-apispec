# Spec Package — OpenAPI Specification Generation

This package generates OpenAPI 3.1 specifications from Go code metadata. It is framework-agnostic and pattern-driven.

## Architecture

```
config.go          Pattern types + framework defaults
  ↓
extractor.go       Unified visitor + pattern matching → RouteInfo
  ↓
mapper.go          RouteInfo → OpenAPI schemas + paths
  ↓
openapi.go         OpenAPI 3.1 type definitions
```

### Key Files

| File | Purpose |
|------|---------|
| `config.go` | `BasePattern`, 6 pattern types, `APISpecConfig`, 6 framework defaults |
| `extractor.go` | `Extractor`, `visitChildren` visitor, response/request/param/content-type extraction, interface resolution, conditional method detection |
| `pattern_matchers.go` | `baseMatchNode`, `basePriority`, Route/Mount/Request matchers |
| `mapper.go` | `MapMetadataToOpenAPI`, schema generation, generic struct instantiation, shortening, disambiguation |
| `type_utils.go` | `sharedResolveTypeOrigin` — single type resolution function for all matchers |
| `tracker.go` | `TrackerTree` — transforms flat call graph into traversable tree |
| `openapi.go` | OpenAPI 3.1 struct types (`OpenAPISpec`, `PathItem`, `Operation`, `Schema`, etc.) |
| `schema_mapper.go` | Low-level Go type → OpenAPI type mapping |
| `visualization.go` | Cytoscape.js call graph diagram generation |
| `export.go` | HTML/JSON export for diagrams |

### Pattern System

All pattern types embed `BasePattern` with shared matching fields:

```go
type BasePattern struct {
    CallRegex, FunctionNameRegex, RecvType, RecvTypeRegex string
    CallerPkgPatterns, CallerRecvTypePatterns             []string
    CalleePkgPatterns, CalleeRecvTypePatterns             []string
}
```

6 pattern types: `RoutePattern`, `RequestBodyPattern`, `ResponsePattern`, `ParamPattern`, `MountPattern`, `ContentTypePattern`.

All 5 matcher implementations delegate to `baseMatchNode()` and `basePriority()` — zero duplicated matching logic.

### Extraction Pipeline

The `Extractor` uses a unified visitor (`visitChildren`) with registered callbacks:

```go
callbacks := []ExtractionCallback{
    routeDetection,
    requestExtraction,
    responseExtraction,
    paramExtraction,
    contentTypeDetection,
    mapIndexParamExtraction,
}
e.visitChildren(node, route, callbacks)
```

Adding a new extraction type requires only registering a callback — no traversal code.

### CFG Integration

Call graph edges and assignments carry `BranchContext` from `golang.org/x/tools/go/cfg`:

```go
type BranchContext struct {
    BlockIndex    int32
    BlockKind     string   // "if-then", "if-else", "switch-case"
    CaseValues    []string // e.g., ["GET", "POST"] for switch cases
}
```

This enables conditional HTTP method detection (`switch r.Method`) and branch-aware analysis.

### Type Resolution

`sharedResolveTypeOrigin()` in `type_utils.go` is the single function for resolving argument types across all matchers. It checks:
1. `arg.GetResolvedType()` — direct resolution
2. Generic type parameter maps
3. Assignment maps for variable tracing
4. Constant return values for cross-function resolution

### Interface Resolution

When a route handler is an interface method:
1. `isInterfaceHandler()` checks metadata for interface types
2. `resolveInterfaceHandler()` searches the call graph for concrete implementations
3. Response patterns are matched against concrete method's edges

### Templates

HTML templates for call graph visualization (in `templates/` subdirectory):
- `templates/cytoscape_template.html` — main interactive diagram
- `templates/paginated_template.html` — paginated version for large graphs
- `templates/server_template.html` — `apidiag` server template

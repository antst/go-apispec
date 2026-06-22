# Changelog

All notable changes to this project are documented here. This project follows
[Semantic Versioning](https://semver.org/). Per the project constitution
(Principle VI), output changes that are *strictly more accurate* are MINOR, not
breaking.

## [Unreleased] — TypeRef Metadata Integration (Phase 2)

### Added — more accurate schema output (MINOR)

The analyzer now builds a structured type model (`TypeRef`) at the AST boundary
and the OpenAPI schema generator walks that tree instead of re-parsing flattened
type strings. This corrects several cases that were previously lossy:

- **Fixed-length arrays** emit `minItems`/`maxItems` — `[3]int`, `[4]Point`, and
  `[16]byte` (a byte ARRAY marshals as a JSON array of integers; only a `[]byte`
  SLICE is a base64 string). Slices stay unconstrained.
- **Multi-parameter generics** (`Pair[K, V]`, `Result[T, E]`) resolve to a
  concrete schema instead of the empty type string the flattener produced.
- **Generic envelope substitution** binds the concrete argument into every
  parameter-typed field **by declared name** (so `Pair[K,V]` maps the first
  argument to `K`, not the positional `T`), not by position.
- **Inline anonymous structs** describe their properties — directly, as a slice
  element (`[]struct{…}`), and as a fixed-array element (which also carries its
  length).
- Removed spurious orphan components for primitive generic arguments
  (`Pair[string,int]` no longer emits `T-string`/`U-int`).

Each item above manifests only when the construct is present and ships with its
own targeted fixtures; the existing framework golden corpus does not exercise
these constructs, so it — and the determinism suite — stays byte-identical.

### Changed — internals (pure refactors; golden-neutral on the existing corpus)

- The string-based schema generator (`mapGoTypeToOpenAPISchema`) and the
  type-string parsing helpers (`TypeParts`, `typeByName`) were deleted; all schema
  generation flows through the `TypeRef` tree, with `ParseTypeRef` recovering a
  tree for the remaining string-only producers. `getTypeName` is retained only for
  the call graph and the body/parameter type path, not schema derivation.

### Changed — internals, Phase 3: resolution-subsystem TypeRef threading (no output change)

The type-**resolution** subsystem now carries a structured `*TypeRef` alongside
the resolved type string, so a resolved request/response/parameter type reaches
the schema generator as a tree instead of being re-parsed from its string. The
OpenAPI corpus is **byte-identical** (the threaded ref always equals
`ParseTypeRef` of the resolved string, so threading it is identical to the
re-parse it replaces).

- `sharedResolveTypeOrigin`, the three `resolveTypeOrigin` matchers, and
  `resolveParamArgType` now return `(string, *TypeRef)`; the parse that used to
  happen inside schema generation moved to this resolution boundary.
  `CallArgument` gained a `ResolvedTypeRef` kept in lockstep with `ResolvedType`
  by `SetResolvedType`; `RequestInfo`/`ResponseInfo` gained a `BodyTypeRef`
  carrier threaded into `schemaForType`. `schemaFromParsedString` remains the
  sole schema-layer re-parse and is now reached only for string-only callers and
  the documented `goType != ref.String()` divergence.
- Removed the dead `TypeResolverImpl` (`type_resolver.go`, the `TypeResolver`
  interface, and the inert `typeResolver` field/constructor parameters) — it
  became unreachable when Phase 2 moved schema generation onto the `TypeRef`
  tree, so its removal changes no output.

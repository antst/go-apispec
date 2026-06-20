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

- **Fixed-length arrays** emit `minItems`/`maxItems` (and `maxLength` for
  `[N]byte`), e.g. `[3]int`, `[16]byte`, `[4]Point` — slices stay unconstrained.
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

### Changed — internals (no output change)

- The string-based schema generator (`mapGoTypeToOpenAPISchema`) and the
  type-string parsing helpers (`TypeParts`, `typeByName`) were deleted; all schema
  generation flows through the `TypeRef` tree, with `ParseTypeRef` recovering a
  tree for the remaining string-only producers. `getTypeName` is retained only for
  the call graph and the body/parameter type path, not schema derivation.

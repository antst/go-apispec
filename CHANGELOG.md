# Changelog

All notable changes to this project are documented here. This project follows
[Semantic Versioning](https://semver.org/). Per the project constitution
(Principle VI), output changes that are *strictly more accurate* are MINOR, not
breaking.

## [Unreleased] — Control-Flow Graph Foundation

### Added — more accurate conditional analysis (MINOR)

The per-function control-flow graph (previously used only to annotate branches)
is now retained as a compact, queryable reachability + dominance model, and the
conditional-analysis consumers resolve over it instead of a source-text-position
heuristic. This sharpens several cases:

- **Conditional status codes** (the #39 / #50 / #57 pain) are computed
  structurally: a status contributes iff its assignment can *reach* the response
  write and is not overwritten on every path by a later, call-dominating
  assignment. Mutually-exclusive `if`/`else` (and `switch`) arms fan out; an
  early-`return` before the write is excluded; an unconditional overwrite shadows
  earlier assignments. **A value assigned inside a loop body now reaches a write
  after the loop** (the analysis terminates across the back-edge).
- **Helper-internal type-switch binding**: when a handler funnels a value into a
  shared responder that `switch v.(type)`s on it, the call-site argument is bound
  to the matching arm — `Respond(w, &NotFound{})` fans out only that arm's status,
  not every arm. When the argument's concrete type is not statically known (a bare
  `error`/`any`), the analyzer degrades to the unconditionally-reachable result
  and emits a warning rather than over-approximating.
- **Method dispatch via `if r.Method == http.MethodPost`** now splits into one
  operation per method, the same as a `switch r.Method` already did.
- **Branch-dependent response bodies** are attributed to the status on which they
  are written (e.g. `FullUser`/200 vs `ErrorBody`/404), never merged.

Each behavior ships with its own targeted fixture (`cfg_helper_typeswitch`,
`cfg_loop_status`, `cfg_method_if_dispatch`, `cfg_branch_bodies`); the existing
framework golden corpus does not exercise these constructs, so it — and the
determinism suite — stays byte-identical.

### Changed — internals

- The conditional-status fan-out's source-position heuristic (`positionAfter`,
  `positionLineCol`, and the "last unconditional index") was **removed** in favor
  of the structural reachability predicate. When control flow cannot be modeled,
  the analyzer falls back to the unconditionally-reachable (single-path) result
  and warns — it never guesses a conditional set.

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

### Changed — internals, Phase 4: authoritative resolved ref (no output change)

The resolved `*TypeRef` is now kept in lockstep with the body/param type string
through every post-resolution transform, replacing the blanket reconcile that
re-derived it from the final string. Pointer dereference unwraps the ref
structurally (`derefPointerRef`), the generic raw-arg and the bound ParamArgMap
arg (research D6) source their ref natively from `arg.TypeRef`, and only genuine
string-origin boundaries (a literal's primitive type, a helper's call-site
recovery) still parse. The OpenAPI corpus is byte-identical.

- **Architectural boundary reached.** A *fully* ref-native resolution subsystem
  (every string derived from a ref) is provably equivalent to changing output
  naming: the only cases where a type string and `ParseTypeRef(string).String()`
  disagree are the non-canonical strings whose canonicalisation alters the
  emitted names. `schemaForType` therefore retains one re-parse for non-canonical
  resolved strings, and the type-parameter map stays `map[string]string` — both
  are load-bearing for the byte-identical contract. Closing them is a deliberate
  output change, out of scope here.

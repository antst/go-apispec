# Configuration Reference

go-apispec auto-detects your framework and infers routes, types, and schemas with
**no configuration**. A config file is only needed to override OpenAPI metadata,
map custom types, scope the analysis, or teach the analyzer a non-standard router.

- **Config file**: pass `--config apispec.yaml` (`-c`). It is parsed by
  `spec.LoadAPISpecConfig` and merged *over* the auto-detected framework defaults
  — you only specify what you want to change.
- **Inspect the effective config**: `apispec --output-config effective.yaml` writes
  the fully-merged config (your file + detected framework defaults) so you can see
  exactly what the analyzer used.

## Precedence

From highest to lowest:

1. A `*spec.APISpecConfig` passed directly to the programmatic API.
2. Your `--config` file (merged **per section** over framework defaults — a section
   you provide replaces that section's default; a section you omit is inherited).
3. Auto-detected framework defaults (`Default<Framework>Config()`).

CLI metadata flags (`--title`, `--api-version`, license/contact, …) fill the
`info` block. `--short-names` overrides `shortNames` **only when the flag is
explicitly passed**.

## File structure

Every section is optional. A minimal file usually sets just `info` and maybe
`typeMapping`:

```yaml
# apispec.yaml
info:
  title: My API
  version: 2.0.0
  description: Public REST API
  contact:
    name: API Team
    email: api@example.com
  license:
    name: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0

servers:
  - url: https://api.example.com/v2

shortNames: true            # false → fully-qualified operationIds & schema names

defaults:
  requestContentType: application/json
  responseContentType: application/json
  responseStatus: 200

typeMapping:
  - goType: time.Time
    openapiType: { type: string, format: date-time }
  - goType: github.com/google/uuid.UUID
    openapiType: { type: string, format: uuid }

externalTypes:
  - name: go.mongodb.org/mongo-driver/bson/primitive.ObjectID
    openapiType: { type: string }
    description: 24-char hex ObjectID

overrides:
  - functionName: GetUser
    summary: Fetch a user
    responseStatus: 200
    responseType: User
    tags: [users]

securitySchemes:
  bearerAuth:
    type: http
    scheme: bearer
    bearerFormat: JWT

include:
  packages: ["internal/api/**"]
exclude:
  files: ["**/*_gen.go"]
  packages: ["**/mocks/**"]
```

## Sections

### OpenAPI metadata

Passed through to the spec verbatim:

| Key | Type | Notes |
|-----|------|-------|
| `info` | object | `title`, `version`, `description`, `termsOfService`, `contact{name,url,email}`, `license{name,url}`. A `license` block is emitted only when `license.name` is set. |
| `servers` | list | `url`, `description`, `variables`. |
| `security` | list | Global security requirement objects. |
| `securitySchemes` | map | `name → {type, scheme, bearerFormat, name, in, flows, …}`. Supports `http`, `apiKey`, `oauth2`, `openIdConnect`. **Auto-detected** Bearer/Basic/apiKey schemes are merged in automatically; a config entry of the same name wins. |
| `tags` | list | `name`, `description`, `externalDocs`. |
| `externalDocs` | object | `url`, `description`. |

### `shortNames`

Controls how operationIds and schema component names are rendered (see
[Naming modes](#naming-modes)). Default **short** (`true`/unset). Set `false` for
fully-qualified, module-path-prefixed names.

### `defaults`

| Key | Default | Effect |
|-----|---------|--------|
| `requestContentType` | `application/json` | Media type for request bodies when not detected from code. |
| `responseContentType` | `application/json` | Media type for responses when not detected (e.g. from `w.Header().Set("Content-Type", …)`). |
| `responseStatus` | `200` | Status used when a handler writes a body with no explicit code. |

### `typeMapping`

Override the schema for a Go type wherever it appears (including as a struct field
or container element). Match is by the **fully-qualified** Go type name.

```yaml
typeMapping:
  - goType: github.com/shopspring/decimal.Decimal
    openapiType: { type: string, format: decimal }
```

### `externalTypes`

Declare third-party types the analyzer can't see into, so they render as a known
schema instead of an empty object.

```yaml
externalTypes:
  - name: github.com/gin-gonic/gin.H
    openapiType: { type: object, additionalProperties: true }
```

### `overrides`

Per-operation overrides keyed by handler `functionName`. Anything set here wins
over inference (and over doc-comment extraction for `summary`/`description`).

| Key | Effect |
|-----|--------|
| `functionName` | Handler function name to match. |
| `summary`, `description` | Operation text. |
| `responseStatus` | Force a status code. |
| `responseType` | Force the response body type. |
| `tags` | Operation tags. |

### `include` / `exclude`

Gitignore-style glob filters scoping which code is analyzed. **`exclude` wins**
over `include` on a conflict. Each has four lists:

```yaml
include:
  files: []
  packages: ["internal/api/**", "internal/handlers/**"]
  functions: []
  types: []
exclude:
  files: ["**/*_test.go", "**/*_gen.go"]
  packages: ["**/internal/mocks/**"]
```

Note: the CLI also auto-excludes tests and mocks by default
(`--auto-exclude-tests`, `--auto-exclude-mocks`) and restricts analysis to
framework-reachable packages (`--auto-include-framework-packages`).

### `framework` (advanced)

The pattern engine that maps Go calls to routes/params/bodies/responses. You
**rarely need this** — the built-in `Default<Framework>Config()` patterns cover
chi, gin, echo, fiber, gorilla/mux, and net/http, and multiple detected frameworks
are merged automatically. Provide a `framework` block only to support a custom
router or tweak a pattern. A section you specify **replaces** that section's
default; omitted sections are inherited.

```yaml
framework:
  routePatterns:
    - callRegex: "^(?i)(GET|POST|PUT|DELETE|PATCH)$"
      recvTypeRegex: "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$"
      handlerArgIndex: 1
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true
  requestBodyPatterns: []
  responsePatterns: []
  paramPatterns: []
  mountPatterns: []
  contentTypePatterns: []
```

Each pattern embeds a common matcher (`BasePattern`):

| Key | Matches against |
|-----|-----------------|
| `callRegex` | The called function/method name. |
| `functionNameRegex` | The enclosing function's name. |
| `recvType` / `recvTypeRegex` | The fully-qualified receiver type (e.g. `github.com/go-chi/chi/v5.*Mux`). The `Regex` form carries `(/v\d)?` in the defaults so versioned import paths match. |
| `callerPkgPatterns` / `calleePkgPatterns` | Caller/callee package globs. |
| `callerRecvTypePatterns` / `calleeRecvTypePatterns` | Caller/callee receiver-type globs. |

Pattern-specific fields (arg indices, `methodFromCall`, `pathFromArg`,
`paramIn`, `isMount`, …) vary by pattern kind; the authoritative reference is the
`Default<Framework>Config()` source in `internal/spec/config.go`. The fastest way
to author a custom pattern is to dump a framework's defaults with
`--output-config` and edit from there.

## Naming modes

| Mode | `shortNames` | operationId | schema component |
|------|--------------|-------------|------------------|
| **Short** (default) | `true` / unset | `DocumentHandler.GetContent` | `User`, `APIResponse_User` |
| **Legacy / fully-qualified** | `false` | `github.com/org/app/http.Deps.DocumentHandler.GetContent` | module-path-qualified |

Short mode strips the module path and resolves any resulting collisions by adding
back the minimum package segments needed to disambiguate. Legacy mode keeps the
full path (using the internal `-->` separator, normalized to `.` in component
keys). Both modes are deterministic and have their own golden snapshots
(`expected_openapi.json` vs `expected_openapi_legacy.json`).

## Struct-tag configuration

Some behavior is configured in your Go source via struct tags, not the YAML file:

- **`json` tags** drive field names, `omitempty` (→ field is optional), and
  `,string` (→ scalar rendered as string).
- **`validate:` / `binding:` tags** (go-playground/validator, gin binding) map to
  schema constraints: `required` → required; `oneof=a b c` → `enum`;
  `min`/`max`/`gte`/`lte` → numeric bounds or string length; `email`/`uuid`/`uri`
  → `format`; `dive,<rule>` → item constraints on slices.
- **Field-level `apispec` tag** — explicit overrides the flow analysis can't infer:

  ```go
  TagsetID string `json:"tagsetId" apispec:"format=uuid,type=string"`
  ```

- **Struct-level `apispec` tag** on a blank marker field — for constraints that
  span fields (useful for PATCH bodies):

  ```go
  type UpdateProfile struct {
      _           struct{} `apispec:"minProperties=1,anyOf=displayName|bio|avatarURL"`
      DisplayName *string  `json:"displayName,omitempty"`
      Bio         *string  `json:"bio,omitempty"`
      AvatarURL   *string  `json:"avatarURL,omitempty"`
  }
  ```

  `,` separates `key=value` pairs; `|` separates field names inside `anyOf`.

## Related CLI flags

| Flag | Effect |
|------|--------|
| `--config`, `-c` | Path to the YAML config (merged over framework defaults). |
| `--output-config`, `-oc` | Write the fully-merged effective config to a file. |
| `--short-names` | Override `shortNames` (only when explicitly passed). |
| `--title`/`-t`, `--api-version`/`-v`, `--description`/`-D`, `--terms`/`-T`, `--contact-name`/`-N`, `--contact-url`/`-U`, `--contact-email`/`-E`, `--license-name`/`-L`, `--license-url`/`-lu` | Fill the `info` block. |
| `--include-package`/`--exclude-package` (and `-file`/`-function`/`-type` variants) | Repeatable glob filters, equivalent to the `include`/`exclude` sections. |
| `--openapi-version`, `-O` | The `openapi:` version field (default `3.1.1`). |

Run `apispec --help` for the full flag list.

# go-apispec Documentation

Start with the [main README](../README.md) — project overview, quick start, the
full feature list, and CLI usage. These docs go deeper on specific topics.

## Using go-apispec

- **[INSTALLATION.md](INSTALLATION.md)** — install methods (`go install`, source,
  script, pre-built binaries), PATH setup, updating, troubleshooting
- **[CONFIGURATION.md](CONFIGURATION.md)** — complete config reference: every
  field, the two naming modes, type mappings, per-operation overrides,
  include/exclude filtering, struct-tag options, and custom framework patterns

## Internals & contributing

- **[INTERFACE_RESOLUTION.md](INTERFACE_RESOLUTION.md)** — how interface types are
  resolved to concrete implementations (embedded-DI structs and interface-method
  handlers)
- **[RELEASE_WORKFLOW.md](RELEASE_WORKFLOW.md)** — the tag → GitHub Actions release
  pipeline and build-time version injection

## Package documentation

- **[../cmd/apispec/README.md](../cmd/apispec/README.md)** — the CLI
- **[../cmd/apidiag/README.md](../cmd/apidiag/README.md)** — interactive call-graph
  diagram server
- **[../generator/README.md](../generator/README.md)** — programmatic generator API
- **[../internal/metadata/README.md](../internal/metadata/README.md)** — metadata,
  AST, and call-graph extraction
- **[../internal/spec/README.md](../internal/spec/README.md)** — OpenAPI mapping,
  patterns, and schema generation

## Links

- [License](../LICENSE) — Apache License 2.0
- [Contributing](../CONTRIBUTING.md)

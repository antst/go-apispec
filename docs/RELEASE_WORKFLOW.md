# Release Workflow

Releases are automated by [`.github/workflows/release.yml`](../.github/workflows/release.yml).
Pushing a `v*` tag validates, builds cross-platform binaries, and publishes a
GitHub Release with assets and auto-generated notes. Version metadata is injected
at build time via `-ldflags` — **no source files are edited or committed** by the
workflow.

## Cutting a release

```bash
# 1. Create and tag (runs scripts/create-release.sh)
make create-tag VERSION=1.2.3

# 2. Push the tag
git push origin v1.2.3
```

Then watch the **Release** workflow under
[Actions](https://github.com/antst/go-apispec/actions). Tags must be `vMAJOR.MINOR.PATCH`
(semver, `v` prefix).

## What the workflow does

Triggered on `push` of a tag matching `v*`. Two jobs:

1. **`validate`** (Go 1.26): `go test ./...` plus the golden-file regression
   (`go test -run TestGolden ./internal/engine/`). The release job does not run if
   this fails.
2. **`release`** (Go 1.26, `needs: validate`):
   - Extracts `VERSION` (tag minus `v`), `COMMIT`, `BUILD_DATE`, `GO_VERSION`.
   - Builds every platform: `VERSION=… COMMIT=… BUILD_DATE=… GO_VERSION=… make release`.
   - Publishes via `softprops/action-gh-release@v2` with the per-platform binaries,
     `*.sha256` checksums, the `apispec-<version>.tar.gz` archive, and
     `generate_release_notes: true`.

## Version injection

`make release` (and `make build`) compile with:

```makefile
LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.BuildDate=$(BUILD_DATE)' \
          -X 'main.GoVersion=$(GO_VERSION)'
```

These overwrite the defaults in `cmd/apispec/main.go` (`Version = "0.0.1"`,
`Commit/BuildDate/GoVersion = "unknown"`). When built **without** ldflags (e.g.
`go install`), `main.go` falls back to Go's embedded VCS build info at runtime, so
`apispec --version` still reports a sensible value.

## Platforms & assets

Built for Linux, macOS, and Windows (amd64 + arm64). Each release includes the
binaries, their `.sha256` checksums, a `.tar.gz` archive, and auto-generated
release notes.

## Local testing

```bash
# Build with an explicit version stamp
make VERSION=1.2.3 build
./apispec --version

# Produce the full release artifact set locally
make VERSION=0.0.0-test release

# Remove a local test tag
git tag -d v0.0.0-test
```

## Troubleshooting

- **Workflow didn't trigger** — the tag must match `v*` and be pushed
  (`git push origin v1.2.3`).
- **`validate` failed on golden files** — output drifted from the committed
  goldens; regenerate intentionally with
  `go test ./internal/engine/ -run TestUpdateGolden` and commit, or fix the
  regression.
- **Asset upload failed** — the workflow needs `contents: write` (already set in
  `release.yml`); check repo Actions permissions.

# Installation

## Prerequisites

- **Go 1.25 or later** — [install from go.dev](https://go.dev/doc/install)
- **Git** (only for building from source)

## Install

### Go install (recommended)

```bash
go install github.com/antst/go-apispec/cmd/apispec@latest
```

The binary lands in `$(go env GOPATH)/bin` (usually `~/go/bin`). Re-run the same
command to update. Pin a release by replacing `@latest` with a tag, e.g.
`@v1.0.0`.

### From source

```bash
git clone https://github.com/antst/go-apispec.git
cd go-apispec
make install-local   # installs to ~/go/bin, no sudo
# or:  make install   # installs system-wide (needs sudo)
```

`make install-local`/`make install` build with version info injected via ldflags.

### Install script

```bash
curl -sSL https://raw.githubusercontent.com/antst/go-apispec/main/scripts/install.sh | bash -s go-install
```

### Pre-built binaries

Download from the [Releases page](https://github.com/antst/go-apispec/releases) —
Linux/macOS/Windows (amd64 + arm64), each with a `.sha256` checksum.

## PATH

Ensure Go's bin directory is on your `PATH`:

```bash
# Linux/macOS — add to ~/.bashrc or ~/.zshrc
export PATH="$(go env GOPATH)/bin:$PATH"
```

On Windows, add `%USERPROFILE%\go\bin` to your PATH, or call the binary by full
path.

## Verify

```bash
apispec --version
```

Output includes the version, commit, build date, and Go version. Tagged-release
builds show the release version; `go install` from `@latest` embeds VCS info when
available, otherwise reports `latest (go install)`.

## Update / uninstall

```bash
# update (go install)
go install github.com/antst/go-apispec/cmd/apispec@latest

# update (source)
cd go-apispec && git pull && make install-local

# uninstall
go clean -i github.com/antst/go-apispec/cmd/apispec   # go install
make uninstall-local                                  # source, local
make uninstall                                        # source, system-wide
```

## Troubleshooting

- **`command not found: apispec`** — Go's bin dir isn't on your `PATH` (see
  [PATH](#path)); restart your shell after editing the profile.
- **Permission denied** — prefer `make install-local` over `make install`.
- **Wrong Go version** — `go version` must report 1.25+.
- **Build failures** — run `go mod download` and retry; check `go env`.

Still stuck? See the [issues](https://github.com/antst/go-apispec/issues) or the
[main README](../README.md).

## For contributors

```bash
git clone https://github.com/antst/go-apispec.git
cd go-apispec
make deps      # download dependencies
make build     # build ./cmd/apispec
make test      # run the suite
make lint      # golangci-lint
```

See [RELEASE_WORKFLOW.md](RELEASE_WORKFLOW.md) for cutting releases.

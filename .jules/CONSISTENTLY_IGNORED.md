# Consistently Ignored Patterns

## IGNORE: Unpinned Tool Versions

**- Pattern:** Using `latest`, `lts`, or version ranges instead of pinned
versions for tools like `go`, `node`, `golangci-lint`, `actionlint` in
`mise.toml`.
**- Justification:** Builds must be deterministic and reproducible. Unpinned
versions lead to inconsistent environments and potential breakage.
**- Files Affected:** `mise.toml`

## IGNORE: Tool Version Downgrades

**- Pattern:** Lowering versions of linters (`golangci-lint`, `actionlint`) or
runtimes (`go`, `node`) in `mise.toml` without explicit instruction.
**- Justification:** Causes loss of newer linting rules/features and potential
build failures due to incompatible configuration syntax.
**- Files Affected:** `mise.toml`

## IGNORE: Mass Go Dependency Downgrades

**- Pattern:** Extensive changes to `go.mod` and `go.sum` that revert multiple
transitive dependencies to older versions (often caused by running `go mod
tidy` with an outdated Go toolchain).
**- Justification:** Introduces security vulnerabilities, breaks compatibility
with newer code, and undoes previous dependency updates.
**- Files Affected:** `go.mod`, `go.sum`

## IGNORE: Explicit Error Suppression

**- Pattern:** Using `_ = f.Close()` or similar constructs to explicitly ignore
errors, especially from `io.Closer`.
**- Justification:** Violates the strict no-ignored-errors policy. All errors
must be handled or logged.
**- Files Affected:** `**/*.go`

## IGNORE: Centralized SDK Configuration

**- Pattern:** Deleting `mise.toml` files in SDK subdirectories
(`sdk/*/mise.toml`) and moving their configuration tasks (e.g., `install:js`,
`lint:js`) to the root `mise.toml`.
**- Justification:** SDKs must remain self-contained with their own tooling
configuration to maintain modularity and avoid complicating the root project
configuration.
**- Files Affected:** `mise.toml`, `sdk/**/mise.toml`

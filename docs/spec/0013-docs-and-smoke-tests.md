# Docs And Smoke Tests

## Goal

Make the completed MVP understandable and testable by early users and agents.

## User-facing Commands

This spec covers documentation and smoke tests for the full MVP command set:

- `tdc configure`
- `tdc cli <subcommand>`
- `tdc db <subcommand>`
- `tdc fs <subcommand>`
- `tdc organization list-projects`

## Behavior

- Provide install and update documentation for the distribution workflow defined
  in `0012-install-and-update-distribution.md`.
- Provide command documentation generated from or synchronized with CLI help.
- Provide smoke tests for the main user workflows.
- Smoke tests that require live TiDB Cloud credentials must be opt-in.
- Completed requirement files should be moved from `docs/spec/` to
  `docs/spec/done/` only after implementation and verification evidence exists.

## Inputs And Config

Documentation must describe:

- `~/.tdc/config`
- `~/.tdc/credentials`
- profile selection
- environment variables
- cloud provider and region selection
- DB SQL user preparation and SQL query modes
- `tdc fs` resource API key storage and redaction behavior
- default JSON output
- `--query`
- `--dry-run`
- auth and permission error handling
- telemetry privacy boundaries
- install and update behavior

Smoke tests use explicit environment variables for live credentials and must not
read secrets from hardcoded files.

## Output And Errors

- Install and update documentation must describe the actionable failures defined
  in `0012-install-and-update-distribution.md`.
- Smoke tests must print the command that failed and the high-level reason, but
  not secret values.

## After This Spec

Early users and agents can follow docs to install the MVP, configure a profile,
and run the main workflows end to end:

```bash
tdc cli check-update --output json
tdc configure
tdc organization list-projects
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --dry-run
tdc db prepare-db-query-access --db-cluster-id <cluster-id>
tdc db execute-sql-statement --db-cluster-id <cluster-id> --sql "select 1"
tdc fs check-file-system
tdc fs list-files --path /
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace
```

Docs must make clear that users select AWS or Alibaba Cloud plus a supported
region, and never provide service URLs.

## Implementation Design

- `docs/commands/` or generated command docs are synchronized from Cobra help.
- `scripts/` contains smoke-test entrypoints. Install scripts are covered by
  `0012-install-and-update-distribution.md`.
- `internal/buildinfo` or `internal/version` provides release metadata consumed
  by docs and `--version`.
- Smoke tests are split into local tests, mock API tests, and opt-in live tests.
- Live tests read credentials from environment variables and select provider and
  region through the same config loader as the CLI.

## API Call Chain

This spec adds no new product API. Smoke tests exercise the call chains defined
in earlier specs:

- `tdc cli check-update`: release manifest lookup from
  `0012-install-and-update-distribution.md`.
- `tdc organization list-projects`: `GET /v1beta1/projects`.
- DB lifecycle dry-runs: construct but do not send the cluster mutation request.
- `tdc db prepare-db-query-access`: SQL user list/create/update APIs.
- `tdc db execute-sql-statement`: HTTP SQL `POST https://http-<host>/v1beta/sql`.
- `tdc fs check-file-system`: `GET /v1/status` after fs endpoint resolution exists.
- Representative fs data-plane commands: `/v1/fs/*` protocol endpoints.
- Mount lifecycle: local mount driver plus fs data-plane protocol calls.

## Dependencies And Platform

- No new runtime dependency is required beyond previous specs.
- Build artifacts must exclude `ref/`.
- Default builds must remain cgo-free unless a platform-specific mount artifact
  explicitly opts in behind build tags.

## Dependencies

- `0001-cli-foundation.md` through `0012-install-and-update-distribution.md`.

## Acceptance Criteria

- `go test ./...` passes.
- Install and update docs match the behavior defined in
  `0012-install-and-update-distribution.md`.
- Generated or maintained command docs match implemented help.
- Opt-in smoke tests cover configure/profile loading, organization lookup, DB
  lifecycle dry-run, DB query preparation, HTTP SQL query, fs check,
  representative fs data-plane commands, and mount lifecycle where supported.

## Out Of Scope

- Release automation beyond the install/update workflow already defined in
  `0012-install-and-update-distribution.md`.

---
title: AGENTS.md - tdc development guide for AI coding agents
---

# Repository Overview

tdc is a Go command-line product for TiDB Cloud Starter. It is designed to be
agent-friendly, predictable, scriptable, and safe for automation.

Module: `github.com/Icemap/tdc`
Go version: 1.26.1 (see `go.mod`)

The most important product document is `docs/priciples.md`. Treat that file as
the source of truth for product principles. Requirement specs live in
`docs/spec/`; completed specs are moved to `docs/spec/done/`.

## Current Implementation Status

Implemented:

- CLI foundation from `docs/spec/done/0001-cli-foundation.md`
- Local config and credentials from
  `docs/spec/done/0002-local-config-and-credentials.md`
- Output, query, and dry-run contracts from
  `docs/spec/done/0003-output-error-query-dry-run.md`
- API client auth, authorization, and region routing from
  `docs/spec/done/0004-api-client-auth-and-region-routing.md`
- Organization project listing from
  `docs/spec/done/0005-organization-management.md`
- Starter DB cluster lifecycle from
  `docs/spec/done/0006-starter-db-cluster-lifecycle.md`
- `tdc configure`
- `tdc organization list-projects`
- `tdc db create-db-cluster`
- `tdc db list-db-clusters`
- `tdc db describe-db-cluster`
- `tdc db update-db-cluster`
- `tdc db delete-db-cluster`
- help and version behavior at every command level
- structured JSON/human rendering and JMESPath `--query`
- `--dry-run` on mutating control-plane command placeholders
- TiDB Cloud Digest-auth API client foundation and auth/authz error mapping
- Makefile build/test/e2e workflow

Registered but not implemented yet:

- `tdc cli check-update`
- `tdc cli update`
- `tdc db ...` branch and SQL commands
- `tdc fs ...` remote service calls and data-plane actions

Those commands are placeholders until their corresponding specs are implemented.
Mutating control-plane placeholders may still return the shared dry-run envelope
when invoked with `--dry-run`.

## Reference Code

- `ref/tidbcloud-cli/` is the previous TiDB Cloud CLI implementation. Use it as
  a reference for TiDB Cloud concepts, profile handling, output helpers,
  telemetry, and API client patterns.
- `ref/drive9/` is the filesystem reference implementation. Use it as context
  for filesystem commands, mount behavior, and data-plane semantics. In tdc
  user-facing output, this domain is always called `tdc fs`.
- `ref/serverless-js/` is a reference for the HTTP SQL call shape.

Reference directories are not product source for tdc. They exist only to give
agents context and implementation examples. In main project code, behave as if
`ref/` does not exist:

- Do not import packages from `ref/`.
- Do not add `replace`, workspace, module, script, or build-time dependencies on
  anything under `ref/`.
- Do not make tests depend on code, data, fixtures, or generated artifacts under
  `ref/`.
- Exclude `ref/` from build, test, lint, release, and packaging flows.

Do not rewrite reference directories unless the task explicitly asks for
reference changes.

## Build And Test Commands

Use the Makefile targets:

```bash
make build
make test
make e2e
make live-e2e
make clean
```

`make build` writes the binary to `bin/tdc`.

`make test` runs ordinary Go tests and must not require live cloud credentials.
`make e2e` builds `bin/tdc` and runs black-box tests against the real binary via
`TDC_E2E_BIN`.
`make live-e2e` builds `bin/tdc` and runs the live TiDB Cloud e2e suite using
the `live-e2e` profile by default. Do not add a separate mutating/non-mutating
live target; live e2e is the full live suite.
Live e2e must strictly cover every implemented interface and command for the
current project stage, including real create/update/delete flows when those
commands are implemented. For Starter DB clusters, the live suite creates a
uniquely named `tdc-e2e-*` cluster without a spending limit and deletes only
that cluster. When a service command is implemented, add its real live
verification to `make live-e2e`; do not leave the target at profile,
smoke-test-only, or mock-only coverage.

For focused work, direct Go commands are also fine:

```bash
go test ./...
go test ./internal/config -run TestName
go build ./cmd/tdc
```

Build artifacts are ignored through `.gitignore`. Do not commit binaries.

Formatting should be standard Go formatting via `gofmt`. Do not run formatters
that rewrite unrelated files.

## Project Layout

Current layout:

```text
cmd/tdc/                    CLI entrypoint
internal/api/               shared HTTP API client and service clients
internal/api/endpoints/     provider/region endpoint resolver
internal/api/transport/     Digest/Bearer/debug HTTP transports
internal/apperr/            typed CLI errors and exit-code helpers
internal/auth/              authenticated profile validation and transports
internal/authz/             permission constants and permission errors
internal/cli/               command wiring and command placeholders
internal/config/            profile loading and precedence rules
internal/config/configure/  interactive configure wizard
internal/config/fsresource/ flat tdc fs config key names
internal/config/region/     provider and region validation
internal/config/store/      TOML read/write, file modes, atomic writes
internal/db/                Starter DB cluster use cases
internal/db/validate/       DB flag and request validation helpers
internal/dryrun/            shared dry-run result envelope
internal/output/            structured JSON/human/raw rendering
internal/organization/      organization project command use cases
internal/query/             JMESPath query application
internal/secretinput/       no-echo secret input helper
internal/version/           build version metadata
e2e/                        black-box tests against the compiled binary
docs/priciples.md           product principles and MVP scope source of truth
docs/spec/                  pending requirement specs
docs/spec/done/             completed requirement specs
ref/                        read-only reference implementations
```

Keep one package per directory. Package names should be short, lowercase, and
without underscores.

## CLI Product Rules

Follow these rules unless `docs/priciples.md` is updated:

- The command tree is at most two levels: `tdc <command> [subcommand]`.
- `tdc configure` is the only intentional exception: it is a top-level verb and
  the only interactive command.
- Other top-level commands are nouns such as `cli`, `db`, `fs`, and
  `organization`.
- Use long flags only, for example `--profile` and `--db-cluster-name`.
- Do not add short flags or one-letter aliases. The current CLI rejects short
  flags before invoking Cobra.
- Do not prompt for input except inside `tdc configure`.
- Successful structured control-plane commands output JSON by default.
- Implement DB, organization, and fs control-plane commands through
  `controlPlaneCommandSpec` in `internal/cli`, so normal execution, dry-run,
  output rendering, and query handling stay on the shared path.
- Each control-plane command must declare exactly one `authz.Permission` in its
  command spec. Do not infer permissions from command names or SQL text.
- Mutating control-plane commands support `--dry-run`.
- `--dry-run` must validate local config, credentials, provider, and region
  before reporting a planned mutation.
- Read-only commands reject `--dry-run`.
- Apply `--query` after command execution and before rendering.
- Users provide cloud placement as `cloud_provider` plus `region_code`, never as
  server URLs.
- Every command should be usable by scripts and agents without
  terminal-specific assumptions.
- Help must work as:
  - `tdc help`
  - `tdc <command> help`
  - `tdc <command> <subcommand> help`

## Commands

Implemented command behavior:

- `tdc configure`
- `tdc configure --non-interactive`
- `tdc help`
- `tdc --version`
- `tdc <command> help`
- `tdc <command> <subcommand> help`
- `tdc <command> --version`
- `tdc <command> <subcommand> --version`
- `tdc organization list-projects`
- `tdc organization list-projects --query 'projects[0].id'`
- `tdc organization list-projects --output human`
- `tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id>`
- `tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id> --dry-run`
- `tdc db list-db-clusters`
- `tdc db list-db-clusters --query 'clusters[].id'`
- `tdc db describe-db-cluster --db-cluster-id <cluster-id>`
- `tdc db update-db-cluster --db-cluster-id <cluster-id> --db-cluster-name new-name`
- `tdc db update-db-cluster --db-cluster-id <cluster-id> --monthly-spending-limit-usd-cents 1000 --dry-run`
- `tdc db delete-db-cluster --db-cluster-id <cluster-id> --confirm-db-cluster-name <current-name>`
- `tdc db delete-db-cluster --db-cluster-id <cluster-id> --confirm-db-cluster-name <current-name> --dry-run`

Registered command surface:

- `tdc cli check-update`
- `tdc cli update`
- `tdc organization list-projects`
- `tdc db create-db-cluster`
- `tdc db list-db-clusters`
- `tdc db describe-db-cluster`
- `tdc db update-db-cluster`
- `tdc db delete-db-cluster`
- `tdc db create-db-cluster-branch`
- `tdc db list-db-cluster-branches`
- `tdc db describe-db-cluster-branch`
- `tdc db delete-db-cluster-branch`
- `tdc db prepare-db-query-access`
- `tdc db create-db-connection-string`
- `tdc db execute-sql-statement`
- `tdc fs create-file-system`
- `tdc fs delete-file-system`
- `tdc fs check-file-system`
- `tdc fs copy-file`
- `tdc fs read-file`
- `tdc fs list-files`
- `tdc fs describe-file`
- `tdc fs move-file`
- `tdc fs delete-file`
- `tdc fs create-directory`
- `tdc fs search-file-content`
- `tdc fs find-files`
- `tdc fs mount-file-system`
- `tdc fs unmount-file-system`

Do not rename commands without updating specs, README, e2e tests, and AGENTS.
Any code change that changes user-visible behavior must keep README.md in sync.

## Configuration And Credentials

All tdc local state belongs under `~/.tdc/`.

- `~/.tdc/config` stores non-sensitive TOML values.
- `~/.tdc/credentials` stores sensitive TOML values.
- Both files use profile sections such as `[default]` and `[stage]`.
- The default profile name is `default`.
- The global `--profile` flag selects a profile when explicitly provided.
- `tdc configure` writes `cloud_provider`, `region_code`, `tdc_public_key`, and
  `tdc_private_key`.
- `tdc configure --non-interactive` must not prompt. It reads values from flags
  first, then `TDC_CLOUD_PROVIDER`, `TDC_REGION_CODE`, `TDC_PUBLIC_KEY`, and
  `TDC_PRIVATE_KEY`. Missing values fail with an actionable error.
- For CI/CD, prefer environment variables for private keys over command-line
  secret flags.
- Interactive `tdc configure` must respond to Ctrl+C and surface an
  `interrupted` error with exit code 130.
- The credentials file is restricted to owner read/write permissions where
  POSIX mode bits are meaningful.

Minimum current keys:

```toml
# ~/.tdc/config
[default]
cloud_provider = "aws"
region_code = "us-east-1"

# ~/.tdc/credentials
[default]
tdc_public_key = "..."
tdc_private_key = "..."
```

Future generated `tdc fs` resource credentials also live in
`~/.tdc/credentials`:

```toml
[default]
fs_api_key = "..."
```

Future DB SQL user credentials live outside the main credentials file:

```text
~/.tdc/db_users/<cluster-id>/credentials
```

That file uses role sections:

```toml
[read_only]
username = "prefix.tdc_ro"
password = "..."

[read_write]
username = "prefix.tdc_rw"
password = "..."

[admin]
username = "prefix.tdc_admin"
password = "..."
```

Do not ask users to provide TiDB Cloud API endpoints, filesystem metadata
database URLs, or server URLs. Endpoint selection is an internal resolver
responsibility based on `cloud_provider` and `region_code`. Test-only endpoint
overrides, if added later, must be hidden from ordinary user workflows and must
not be required by MVP usage.

TiDB Cloud control-plane API calls use HTTP Digest auth through
`internal/api/transport`; never send `tdc_private_key` as Basic Auth for those
APIs. SQL HTTP execution and tdc fs data-plane auth are separate authentication
schemes defined by their later specs.

Use `internal/api/endpoints` for Starter, IAM/account, and fs endpoint
selection. Do not add service URLs to user config. The default Starter host is
`https://serverless.tidbapi.com`; the default IAM host is
`https://iam.tidbapi.com`. The default tdc fs host is intentionally unavailable
until product endpoint routing is confirmed.

Credential lookup order for authenticated commands:

1. If `--profile <name>` is explicitly provided, read that profile from
   `~/.tdc/config` and `~/.tdc/credentials`.
2. If no profile is explicitly provided and any credential environment variable
   is present, read environment credentials from `TDC_CLOUD_PROVIDER`,
   `TDC_REGION_CODE`, `TDC_PUBLIC_KEY`, and `TDC_PRIVATE_KEY`. All four are
   required in this mode.
3. Otherwise read the `default` profile.

When implementing command handlers, detect whether `--profile` was explicitly
set before calling `config.Load`; the root flag has a default value, but that
default must not suppress environment-variable fallback.

Supported MVP placement values:

| Cloud provider | Region labels | Region codes |
| --- | --- | --- |
| `aws` | N. Virginia, Oregon, Frankfurt, Tokyo, Singapore | `us-east-1`, `us-west-2`, `eu-central-1`, `ap-northeast-1`, `ap-southeast-1` |
| `alibaba_cloud` | Singapore | `ap-southeast-1` |

Do not store secrets in logs, telemetry, generated docs examples, or test
fixtures.

Generated DB SQL usernames and passwords live in
`~/.tdc/db_users/<cluster-id>/credentials`, not in the main
`~/.tdc/credentials` file. Do not add nested
`[profile.db_users."<cluster-id>".role]` TOML sections to
`~/.tdc/credentials`. TiDB Cloud cluster IDs are globally unique, so DB SQL
credentials are cluster-scoped rather than profile-scoped. `tdc db
prepare-db-query-access` owns those credentials and must be idempotent: it
creates or repairs the stable tdc-managed read-only, read-write, and admin
users for a cluster instead of creating a new group every time.

Generated `tdc fs` resource API keys also live in `~/.tdc/credentials`.
User-facing docs and commands must call these `tdc fs` API keys or resource
credentials, never reference implementation API keys. Filesystem data-plane
requests authenticate with `Authorization: Bearer <tdc-fs-api-key>` after the
resource is created.

`tdc db create-db-connection-string` and `tdc db execute-sql-statement` use
read-write credentials by default. `--read-write`, `--read-only`, and `--admin`
must be mutually exclusive explicit selections. Do not add SQL-text
classification or an automatic access mode.

## Output And Errors

Use structured output contracts from the start.

- JSON is the default for successful structured control-plane commands.
- Data-plane commands may stream bytes or plain file listings when JSON would
  break expected filesystem usage.
- `--output json` and `--output human` are the initial output modes.
- `--query` uses JMESPath semantics and is applied after command execution to
  the structured result.
- Raw output commands must reject `--query`.
- Mutating control-plane commands use `internal/dryrun` for shared `--dry-run`
  envelopes, load the active profile, and must stop before remote mutation.
- API/auth errors must preserve categories and exit codes: `3` authentication,
  `4` authorization, and `5` remote not found.
- Errors follow this shape:

```text
tdc [ERROR]: <actionable message>
```

Library code returns errors instead of printing or exiting. Only the CLI
boundary writes to stdout/stderr and maps errors to exit codes.

## Telemetry Rules

Telemetry is not implemented yet. When implemented, it must be opt-aware and
privacy-preserving. Allowed fields:

- command and subcommand invoked
- flag names used, never flag values
- error codes and execution time
- TiDB Cloud region
- CLI version
- OS type

Never collect credentials, file contents, SQL text, query output, local paths
that can reveal sensitive data, or API response payloads.

## Go Style

- Return errors; do not panic in library code.
- Wrap errors with operation context using `%w`.
- Prefer typed string constants for domain enums.
- Constructors should use `New(...)` or `NewWithConfig(cfg Config)`.
- Test helpers accept `*testing.T` as the first argument and call `t.Helper()`.
- Use standard library facilities unless the project already has a chosen
  dependency for the same purpose.
- Keep command handlers thin; put reusable behavior in internal packages.

Imports should be grouped as standard library, third-party, then internal
packages, separated by blank lines.

## Testing Expectations

For new behavior, add focused tests at the package boundary that owns the
contract.

Current expectations:

- `make test` must pass without live cloud credentials.
- `make e2e` must pass and should exercise the compiled binary, not internal Go
  packages directly.
- Unit tests should use temp home directories for config and credentials.
- E2E tests should use temp `HOME` values and must not touch the user's real
  `~/.tdc/`.
- API client tests should use mock HTTP servers once API specs are implemented.
- Live cloud tests are opt-in, skipped by default, and run through
  `make live-e2e`. They must use the `live-e2e` profile and verify the real
  API/command surface for every implemented spec. Implemented mutating commands
  must have real live mutation coverage with resource names scoped to the test
  run and cleanup that only targets resources created by that run.

Do not require live cloud credentials for ordinary `go test ./...`.

## Documentation Workflow

Pending requirements live in `docs/spec/` and are numbered by dependency order,
for example `0003-output-error-query-dry-run.md`. When a requirement is fully
implemented and verified, move its file to `docs/spec/done/` and mention the
verification evidence in the final response.

README.md is the user-facing source for current usage. After every code change,
check whether README.md still matches the implemented CLI. Update README.md in
the same change whenever commands, flags, config files, environment variables,
build/test commands, error behavior, outputs, or implemented/not-implemented
status changes. Do not leave code and README.md out of sync.

Keep each spec decision-complete for implementation: commands, behavior, inputs,
outputs, dependencies, acceptance criteria, and explicit out-of-scope notes.

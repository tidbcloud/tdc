# tdc

`tdc` is an agent-friendly command-line interface for TiDB Cloud Starter.

The project is in early MVP implementation. The CLI foundation, local
configuration/credentials flow, structured output, JMESPath query, shared
dry-run behavior, and API client/auth foundation are implemented. Service
commands are registered so users and agents can discover the product surface,
but most service actions still return "not implemented" until their specs are
completed.

## Current Status

Implemented:

- `tdc help`
- `tdc --version`
- `tdc <command> help`
- `tdc <command> <subcommand> help`
- `tdc <command> --version`
- `tdc <command> <subcommand> --version`
- `tdc configure`
- local TOML config and credentials under `~/.tdc/`
- JSON and human output rendering for structured command results
- JMESPath `--query` on structured command results
- `--dry-run` on mutating control-plane command placeholders
- TiDB Cloud Digest-auth API client foundation
- provider/region endpoint resolver for Starter and IAM APIs
- permission declarations and auth/authz error categories

Registered but not implemented yet:

- `tdc cli check-update`
- `tdc cli update`
- `tdc db ...` remote service calls
- `tdc fs ...` remote service calls and data-plane actions
- `tdc organization list-projects`

## Build

Requirements:

- Go 1.26.1 or newer
- `make`

Build the local binary:

```bash
make build
```

The binary is written to:

```text
bin/tdc
```

Run tests:

```bash
make test
make e2e
```

Run the live TiDB Cloud e2e entrypoint:

```bash
make live-e2e
```

`make live-e2e` uses the `live-e2e` profile by default. Configure that profile
before running live tests:

```bash
bin/tdc configure --profile live-e2e
```

For CI/CD:

```bash
TDC_CLOUD_PROVIDER=aws \
TDC_REGION_CODE=us-east-1 \
TDC_PUBLIC_KEY="$TDC_PUBLIC_KEY" \
TDC_PRIVATE_KEY="$TDC_PRIVATE_KEY" \
bin/tdc configure --profile live-e2e --non-interactive

make live-e2e
```

At the current implementation stage, `make live-e2e` validates the real binary,
the `live-e2e` profile, real TiDB Cloud Digest-auth read-only API probes, the
current command surface, mutating command dry-runs, and read-only dry-run
rejection. As TiDB Cloud API commands are implemented, their live tests must be
added to this same target.

Clean build artifacts:

```bash
make clean
```

## Quick Start

Build the CLI:

```bash
make build
```

Show help:

```bash
bin/tdc help
bin/tdc db help
bin/tdc fs mount-file-system help
```

Configure the default profile:

```bash
bin/tdc configure
```

Configure a named profile:

```bash
bin/tdc configure --profile stage
```

Configure non-interactively for CI/CD:

```bash
TDC_CLOUD_PROVIDER=aws \
TDC_REGION_CODE=us-east-1 \
TDC_PUBLIC_KEY="$TDC_PUBLIC_KEY" \
TDC_PRIVATE_KEY="$TDC_PRIVATE_KEY" \
bin/tdc configure --profile ci --non-interactive
```

You can also pass values as flags:

```bash
bin/tdc configure \
  --profile ci \
  --cloud-provider aws \
  --region-code us-east-1 \
  --tdc-public-key "$TDC_PUBLIC_KEY" \
  --tdc-private-key "$TDC_PRIVATE_KEY" \
  --non-interactive
```

For CI/CD, prefer environment variables for secrets so private keys do not
appear in shell history or process lists.

`tdc configure` prompts for:

- cloud provider
- region code
- TiDB Cloud public key
- TiDB Cloud private key

The private key is not printed after entry. When stdin is a terminal, private
key input is read without echo. Pressing Ctrl+C interrupts `tdc configure` and
exits with code 130.

## Command Rules

`tdc` is designed for agents and scripts:

- Use long flags only. Short flags such as `-h` are rejected.
- Help is available as an explicit command, for example `tdc db help`.
- Successful structured command results render as JSON by default.
- `--output json` and `--output human` are supported output modes.
- `--query <jmespath-expression>` is applied after command execution and before
  rendering.
- Mutating control-plane commands support `--dry-run`.
- `--dry-run` loads the active profile and validates local config, credentials,
  provider, and region before reporting the planned mutation.
- Read-only commands reject `--dry-run`.
- Authenticated command failures use stable exit codes: `3` for
  authentication, `4` for authorization, and `5` for remote not found.
- Errors are rendered at the CLI boundary as:

```text
tdc [ERROR]: <actionable message>
```

Global flags:

- `--profile <name>`
- `--debug`
- `--output <json|human>`
- `--query <jmespath-expression>`
- `--help`
- `--version`

## Commands

### Root

```bash
tdc help
tdc --version
tdc configure
tdc configure --profile <profile-name>
tdc configure --profile <profile-name> --non-interactive
```

Configure-specific flags:

- `--cloud-provider <aws|alibaba_cloud>`
- `--region-code <region-code>`
- `--tdc-public-key <key>`
- `--tdc-private-key <key>`
- `--non-interactive`

### CLI Management

```bash
tdc cli check-update
tdc cli update
```

These commands are registered but not implemented yet.

### Organization

```bash
tdc organization list-projects
```

This command is registered but not implemented yet.

### DB Cluster

```bash
tdc db create-db-cluster
tdc db create-db-cluster --dry-run
tdc db create-db-cluster --dry-run --output human
tdc db create-db-cluster --dry-run --query command
tdc db list-db-clusters
tdc db describe-db-cluster
tdc db update-db-cluster
tdc db update-db-cluster --dry-run
tdc db delete-db-cluster
tdc db delete-db-cluster --dry-run
```

Remote service calls are not implemented yet. Mutating control-plane commands
currently support the shared `--dry-run` envelope.

### DB Branch

```bash
tdc db create-db-cluster-branch
tdc db create-db-cluster-branch --dry-run
tdc db list-db-cluster-branches
tdc db describe-db-cluster-branch
tdc db delete-db-cluster-branch
tdc db delete-db-cluster-branch --dry-run
```

Remote service calls are not implemented yet. Mutating control-plane commands
currently support the shared `--dry-run` envelope.

### DB SQL

```bash
tdc db prepare-db-query-access
tdc db prepare-db-query-access --dry-run
tdc db create-db-connection-string
tdc db execute-sql-statement
```

These commands are registered but not implemented yet, except that
`prepare-db-query-access --dry-run` returns the shared dry-run envelope.

The intended SQL access model is:

- `prepare-db-query-access` prepares stable tdc-managed DB users.
- `create-db-connection-string` will emit connection strings from prepared
  credentials, including `.env` component output for agents.
- `execute-sql-statement` uses read-write credentials by default.
- `--read-write`, `--read-only`, and `--admin` will be mutually exclusive
  explicit selections once implemented. Read-write remains the default.
- SQL mode selection must not be inferred from SQL text.

### tdc fs Control Plane

```bash
tdc fs create-file-system
tdc fs create-file-system --dry-run
tdc fs delete-file-system
tdc fs delete-file-system --dry-run
tdc fs check-file-system
```

Remote service calls are not implemented yet. Mutating control-plane commands
currently support the shared `--dry-run` envelope.

### tdc fs Data Plane

```bash
tdc fs copy-file
tdc fs read-file
tdc fs list-files
tdc fs describe-file
tdc fs move-file
tdc fs delete-file
tdc fs create-directory
tdc fs search-file-content
tdc fs find-files
```

These commands are registered but not implemented yet.

### tdc fs Mount Runtime

```bash
tdc fs mount-file-system
tdc fs unmount-file-system
```

These commands are registered but not implemented yet.

## Configuration

All local state is stored under:

```text
~/.tdc/
```

Non-sensitive config:

```text
~/.tdc/config
```

Sensitive TiDB Cloud and tdc fs credentials:

```text
~/.tdc/credentials
```

The credentials file is restricted to owner read/write permissions where the
platform supports POSIX mode bits.

Minimum config shape:

```toml
[default]
cloud_provider = "aws"
region_code = "us-east-1"
```

Minimum credentials shape:

```toml
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

Future DB SQL user credentials live in a cluster-scoped credentials file:

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

The DB SQL user credential path is not profile-scoped because TiDB Cloud
cluster IDs are globally unique. The active profile only controls which TiDB
Cloud API keys are used to prepare or repair those credentials. Do not add
`[default.db_users."cluster-id".role]` sections to `~/.tdc/credentials`; the CLI
rejects that legacy shape.

Do not configure TiDB Cloud API URLs, filesystem server URLs, metadata database
URLs, or endpoint overrides in normal user config. Endpoint resolution is an
internal responsibility derived from `cloud_provider` and `region_code`.

## API Auth And Endpoints

TiDB Cloud control-plane requests use HTTP Digest authentication with
`tdc_public_key` as the digest username and `tdc_private_key` as the digest
password. The private key is not sent as Basic Auth.

Endpoint routing is internal:

- Starter API: `https://serverless.tidbapi.com`
- IAM/account API: `https://iam.tidbapi.com`
- tdc fs API: resolved by provider/region once the product endpoint contract or
  discovery API is confirmed

Each control-plane command declares a permission requirement internally. Remote
APIs remain the source of truth for the actual permission decision.

## Profile And Environment Lookup

Authenticated commands use this lookup order:

1. If `--profile <name>` is explicitly provided, read that profile from
   `~/.tdc/config` and `~/.tdc/credentials`.
2. If no profile is explicitly provided and any `TDC_*` credential environment
   variable is present, read environment credentials.
3. Otherwise read the `default` profile.

Environment variables:

```bash
TDC_CLOUD_PROVIDER=aws
TDC_REGION_CODE=us-east-1
TDC_PUBLIC_KEY=...
TDC_PRIVATE_KEY=...
```

When environment credentials are used, all four variables are required.

## Supported Cloud Placement

Users provide cloud provider and region code, never service URLs.

| Cloud provider | Region label | Region code |
| --- | --- | --- |
| `aws` | N. Virginia | `us-east-1` |
| `aws` | Oregon | `us-west-2` |
| `aws` | Frankfurt | `eu-central-1` |
| `aws` | Tokyo | `ap-northeast-1` |
| `aws` | Singapore | `ap-southeast-1` |
| `alibaba_cloud` | Singapore | `ap-southeast-1` |

## Development Notes

Reference code under `ref/` is for context only. It is not imported, linked,
tested against, or packaged as part of `tdc`.

Completed requirement specs are moved to:

```text
docs/spec/done/
```

Pending requirement specs remain in:

```text
docs/spec/
```

# tdc

`tdc` is an agent-friendly command-line interface for TiDB Cloud Starter.

The project is in early MVP implementation. The CLI foundation, local
configuration/credentials flow, structured output, JMESPath query, shared
dry-run behavior, and API client/auth foundation are implemented. Service
commands are registered so users and agents can discover the product surface.
Organization project listing, Starter DB cluster lifecycle commands, and
Starter DB branch, SQL access, and tdc fs control-plane commands are
implemented. tdc fs data-plane file commands are implemented. tdc fs mount
actions still return "not implemented" until their specs are completed.

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
- `--dry-run` on mutating control-plane commands and placeholders
- TiDB Cloud Digest-auth API client foundation
- provider/region endpoint resolver for Starter and IAM APIs
- permission declarations and auth/authz error categories
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

Registered but not implemented yet:

- `tdc cli check-update`
- `tdc cli update`
- `tdc fs mount-file-system`
- `tdc fs unmount-file-system`

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
the `live-e2e` profile, real TiDB Cloud Digest-auth read-only API probes,
`tdc organization list-projects`, the current command surface, mutating command
dry-runs, read-only dry-run rejection, a full tdc fs data-plane lifecycle, and
the full Starter DB cluster, SQL access, and branch lifecycles. The live tdc fs
lifecycle writes only under a unique `/tdc-e2e-*` path, uploads a local file,
lists/describes/reads/searches/finds it, performs remote copy/move, downloads it
back, and deletes the test path recursively. The live DB lifecycle creates one
uniquely named `tdc-e2e-*` Starter cluster without a spending limit, prepares
tdc-managed read-only/read-write/admin SQL users, creates connection strings,
executes HTTP SQL with all three access modes, creates one `tdc-e2e-branch-*`
branch on that cluster, lists/describes/deletes the branch, updates the cluster,
reads it again, deletes it, and verifies deletion. As TiDB Cloud API commands
are implemented, their real live tests must be added to this same target.

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
tdc organization list-projects --page-size 10
tdc organization list-projects --page-token <next-page-token>
tdc organization list-projects --query 'projects[0].id'
tdc organization list-projects --output human
```

This command calls the TiDB Cloud IAM/account API with the active profile's
Digest-auth API key pair and returns the projects visible to that profile.

### DB Cluster

```bash
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id>
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id> --monthly-spending-limit-usd-cents 1000
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id> --dry-run
tdc db list-db-clusters
tdc db list-db-clusters --page-size 10
tdc db list-db-clusters --query 'clusters[].id'
tdc db describe-db-cluster --db-cluster-id <cluster-id>
tdc db describe-db-cluster --db-cluster-id <cluster-id> --view FULL
tdc db update-db-cluster --db-cluster-id <cluster-id> --db-cluster-name new-name
tdc db update-db-cluster --db-cluster-id <cluster-id> --monthly-spending-limit-usd-cents 1000 --dry-run
tdc db delete-db-cluster --db-cluster-id <cluster-id> --confirm-db-cluster-name <current-name>
tdc db delete-db-cluster --db-cluster-id <cluster-id> --confirm-db-cluster-name <current-name> --dry-run
```

These commands call the TiDB Cloud Starter API with the active profile's
Digest-auth API key pair. Create requires `--db-cluster-type starter` and a
`--project-id`; discover project ids with `tdc organization list-projects`.
Cluster JSON output uses stable snake_case fields such as `id`, `display_name`,
and `next_page_token`.

Delete is non-interactive. Normal execution reads the remote cluster first and
requires `--confirm-db-cluster-name` to exactly match the current remote display
name before sending the delete request.

### DB Branch

```bash
tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev
tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev --dry-run
tdc db list-db-cluster-branches --db-cluster-id <cluster-id>
tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --page-size 10
tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --query 'branches[].id'
tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --output human
tdc db describe-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id>
tdc db describe-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --view FULL
tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --confirm-db-cluster-branch-name dev
tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --confirm-db-cluster-branch-name dev --dry-run
```

These commands call the TiDB Cloud Starter branch API with the active profile's
Digest-auth API key pair. Create currently sends the API-backed `displayName`
field through `--db-cluster-branch-name`.

Delete is non-interactive. Normal execution reads the remote branch first and
requires `--confirm-db-cluster-branch-name` to exactly match the current remote
display name before sending the delete request.

### DB SQL

```bash
tdc db prepare-db-query-access --db-cluster-id <cluster-id>
tdc db prepare-db-query-access --db-cluster-id <cluster-id> --dry-run
tdc db create-db-connection-string --db-cluster-id <cluster-id>
tdc db create-db-connection-string --db-cluster-id <cluster-id> --read-write --format mysql-uri
tdc db create-db-connection-string --db-cluster-id <cluster-id> --read-only --format env
tdc db create-db-connection-string --db-cluster-id <cluster-id> --admin --format jdbc
tdc db execute-sql-statement --db-cluster-id <cluster-id> --sql "select 1"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-write --sql "insert into t values (1)"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-only --sql "select * from t"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --admin --sql "show grants"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --transport mysql --sql "select 1"
```

`prepare-db-query-access` creates or repairs three stable tdc-managed SQL
users for the cluster:

- `read_only`, backed by TiDB Cloud role `role_readonly`
- `read_write`, backed by TiDB Cloud role `role_readwrite`
- `admin`, backed by TiDB Cloud role `role_admin`

Generated DB SQL usernames and passwords are stored under:

```text
~/.tdc/db_users/<cluster-id>/credentials
```

Re-running `prepare-db-query-access` is idempotent. It does not create a new
user group when the tdc-managed users already exist. If local passwords are
missing for verified tdc-managed remote users, it rotates those passwords
through the SQL user API and writes the new local credentials.

`create-db-connection-string` and `execute-sql-statement` use read-write
credentials by default. `--read-write`, `--read-only`, and `--admin` are
mutually exclusive explicit selections. tdc never infers access mode from SQL
text.

Connection string formats:

- `mysql-uri`
- `jdbc`
- `go-sql-driver`
- `sqlalchemy`
- `env`

`--format env` emits dotenv-compatible component variables directly, not JSON,
so agents can compose framework-specific values without parsing URLs. Use
`--env-include-database-url` to include a `DATABASE_URL`-style value.

`execute-sql-statement` executes exactly one SQL statement per invocation. HTTP
SQL is the default transport and uses `POST https://http-<cluster-host>/v1beta/sql`
with Basic Auth from the prepared SQL credentials. `--transport mysql` is an
explicit fallback that opens one MySQL connection, executes once, and closes it.
Use `--output human` to render result sets as a terminal table. JSON remains
the default output for agents and automation.

### tdc fs Control Plane

```bash
tdc fs check-file-system
tdc fs create-file-system --file-system-name workspace --dry-run
tdc fs create-file-system --file-system-name workspace
tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace --dry-run
tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace
```

`create-file-system` and `delete-file-system` are wired through the tdc fs
control-plane client. Endpoint routing uses the hosted tdc fs region manifest
and matches the active profile's `cloud_provider + region_code` against
`tidb_cloud_native` entries. Users never provide a raw server URL. If the
manifest does not include the profile placement, the command returns a clear
unsupported-region error.

`create-file-system` provisions with the profile's TiDB Cloud API key pair in
the HTTPS request body expected by the tdc fs backend. `delete-file-system`
uses the stored `fs_api_key` as Bearer auth and also sends the TiDB Cloud key
pair required for native tenant deletion. `--dry-run` validates config and
shows a redacted request shape without printing credential values.

`create-file-system` stores returned resource metadata as flat `fs_*` keys in
`~/.tdc/config` and stores the returned API key as `fs_api_key` under the active
profile in `~/.tdc/credentials`. The API key is not printed in command output.
`delete-file-system` clears the flat `fs_*` config and credential keys only
after remote deletion succeeds.

`check-file-system` returns structured check status for local config,
credentials, endpoint resolution, and remote service reachability. If
`fs_api_key` has not been created yet, remote status is reported as a warning
instead of making an unauthenticated `/v1/status` call. If the manifest does not
support the configured placement, endpoint selection is reported as failed.

### tdc fs Data Plane

```bash
tdc fs copy-file --from-local ./README.md --to-remote /workspace/README.md
tdc fs copy-file --from-remote /workspace/README.md --to-local ./README.copy.md --create-parents
tdc fs copy-file --from-remote /workspace/README.md --to-remote /workspace/README.copy.md
tdc fs read-file --path /workspace/README.md
tdc fs list-files --path /workspace
tdc fs list-files --path /workspace --output human
tdc fs describe-file --path /workspace/README.md
tdc fs move-file --from-remote /workspace/README.copy.md --to-remote /workspace/archive/README.md
tdc fs delete-file --path /workspace/archive/README.md
tdc fs delete-file --path /workspace/archive --recursive
tdc fs create-directory --path /workspace/archive --mode 0755
tdc fs search-file-content --path /workspace --pattern "hello"
tdc fs find-files --path /workspace --file-name-pattern "*.md"
```

These commands use the active profile's stored `fs_api_key` as Bearer auth and
call the tdc fs data-plane endpoint selected from the hosted region manifest.
Run `tdc fs create-file-system` before using them, or configure the flat
`fs_api_key` credential manually if the resource already exists.

`read-file` writes raw file bytes to stdout and does not wrap the response in
JSON. Do not combine `read-file` with `--query`; queries require structured
output. Metadata and search commands return structured JSON by default and
support `--output human` for terminal tables.

`copy-file` supports exactly one explicit source/target pair:
`--from-local` with `--to-remote`, `--from-remote` with `--to-local`, or
`--from-remote` with `--to-remote`. Remote and local targets are not overwritten
unless `--overwrite` is provided. `--create-parents` only applies when copying
from tdc fs to a local path.

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

Profile selection order:

1. Non-empty `--profile <name>`
2. `TDC_PROFILE=<name>`
3. `default`

An explicit empty `--profile ""` is a usage error. Omitting `--profile` is not
an error: the CLI uses `TDC_PROFILE` when it is set, otherwise it uses
`default`. For shell scripts and CI jobs, prefer either a literal
`--profile live-e2e` or an exported `TDC_PROFILE=live-e2e`.

Generated `tdc fs` resource credentials also live in `~/.tdc/credentials` as a
flat key under the active profile:

```toml
[default]
fs_api_key = "..."
```

Generated non-secret `tdc fs` resource metadata lives in `~/.tdc/config` as
flat keys under the active profile:

```toml
[default]
fs_resource_name = "workspace"
fs_tenant_id = "tenant-..."
fs_cloud_provider = "aws"
fs_region_code = "us-east-1"
```

DB SQL user credentials live in a cluster-scoped credentials file:

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

SQL HTTP execution uses the prepared DB SQL username and password as HTTP Basic
Auth against `https://http-<cluster-host>/v1beta/sql`. Do not confuse these DB
credentials with TiDB Cloud API keys.

Endpoint routing is internal:

- Starter API: `https://serverless.tidbapi.com`
- IAM/account API: `https://iam.tidbapi.com`
- tdc fs API: resolved from the hosted tdc fs region manifest, currently using
  `tidb_cloud_native` endpoint entries

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

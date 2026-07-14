# tdc

`tdc` is an agent-friendly command-line interface for TiDB Cloud Starter.

The initial MVP command surface is implemented. It covers local configuration and credentials, structured output, JMESPath query, shared dry-run behavior, TiDB Cloud auth and region routing, organization project listing, Starter DB lifecycle, SQL access, tdc fs control/data plane, FUSE/WebDAV mount runtime, layer workflows, vault workflows, journal workflows, Drive9-style Git workspace workflows, installer/update flows, and CI/live-e2e release automation.

## Install

GitHub Releases are the MVP distribution channel. macOS and Linux users can install the latest release with:

```bash
curl -fsSL https://github.com/tidbcloud/tdc/releases/latest/download/install.sh | sh -s -- --yes
```

Install a pinned release:

```bash
curl -fsSL https://github.com/tidbcloud/tdc/releases/download/v0.1.0/install.sh | sh -s -- --version v0.1.0 --yes
```

Windows users can use the PowerShell installer:

```powershell
$script = "$env:TEMP\install-tdc.ps1"
iwr https://github.com/tidbcloud/tdc/releases/latest/download/install.ps1 -OutFile $script
powershell -ExecutionPolicy Bypass -File $script -InstallDir "$HOME\bin" -Yes
```

The installers download the matching archive, verify `tdc_checksums.txt`, install the binary, and run `tdc --version`. Without `--install-dir`, the macOS/Linux installer upgrades the active `tdc` binary found on `PATH`; when no active binary exists, it installs to `/usr/local/bin` and uses `sudo` when that directory is not writable. The Windows installer uses the active `tdc.exe` directory when one exists, otherwise `$HOME\bin`.

After installation, the scripts detect PATH shadowing, bootstrap `~/.tdc/config` with `[default] region_code = "aws-us-east-1"` only when that file is missing, print TiDB Cloud DB config regions, fetch and print the current tdc fs region manifest when available, and show next-step commands. Installers never write `~/.tdc/credentials`.

Check for updates:

```bash
tdc update --check
tdc update --check --output text
```

Update an official archive/script install:

```bash
tdc update --dry-run
tdc update --yes
tdc update --target-version v0.1.0 --yes
```

Local `make build` binaries are marked as `install_source=local`; `tdc update` refuses them. Windows self-update cannot safely replace the running executable yet, so rerun `install.ps1` there. Homebrew and Scoop are planned in `docs/spec/0016-homebrew-and-scoop-distribution.md`; apt/yum/winget and other reviewed package channels are not part of the current plan.

## Build

Requirements:

- Go 1.26.1 or newer
- `make`
- GoReleaser, only for `make release-snapshot` or release publishing

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

Build local release artifacts without publishing:

```bash
make release-snapshot
```

Release publishing uses GoReleaser and GitHub Releases. Before pushing a `v*` tag, write a summary-style release note at `docs/release-notes/<tag>.md`, for example `docs/release-notes/v0.1.0.md`. The release workflow passes that file to GoReleaser with `--release-notes`, and fails when the tag-specific file is missing so GitHub Releases do not fall back to a commit-list changelog.

Run the live TiDB Cloud e2e entrypoint:

```bash
make live-e2e
```

`make live-e2e` uses the `live-e2e` profile by default. Configure that profile before running live tests:

```bash
bin/tdc configure --profile live-e2e
```

For CI/CD:

```bash
TDC_REGION_CODE=aws-us-east-1 \
TDC_PUBLIC_KEY="$TDC_PUBLIC_KEY" \
TDC_PRIVATE_KEY="$TDC_PRIVATE_KEY" \
bin/tdc configure --profile live-e2e --non-interactive

make live-e2e
```

At the current implementation stage, `make live-e2e` validates the real binary, the `live-e2e` profile, real TiDB Cloud Digest-auth read-only API probes, `tdc organization list-projects`, the current command surface, mutating command dry-runs, read-only dry-run rejection, a full tdc fs data-plane lifecycle, and the full Starter DB cluster, SQL access, and branch lifecycles. If the `live-e2e` profile has no `fs_api_key`, the live suite creates a temporary tdc fs resource named by `TDC_LIVE_FS_NAME` or `workspace`, stores the generated flat `fs_*` metadata and `fs_api_key`, and deletes that auto-created resource when the test process exits. The live tdc fs lifecycle writes only under a unique `/tdc-e2e-*` path, uploads local files, verifies multipart upload, efficient append, upload resume, range reads, lists/describes/reads/searches/finds files, performs remote copy/move, verifies stdin/stdout copy, tags/descriptions, chmod, symlink, hardlink, pack/unpack, low-level Git workspace APIs, creates and commits a real tdc fs layer, creates/reads/replaces/deletes a real tdc fs-vault secret, verifies delegated grant/token reads, mounts the vault and reads a field through the mounted filesystem on macOS/Linux, lists vault audit events, creates/appends/reads/searches/verifies a real tdc fs-journal, downloads content back, and deletes the test path recursively. On macOS or Linux with FUSE available, live e2e also mounts a unique remote path through the default FUSE driver, reads and writes through the local mount, drains it, unmounts it, and cleans up the remote path. On macOS with `mount_webdav` available, live e2e also verifies the explicit `--driver webdav` fallback path. The live DB lifecycle creates one uniquely named `tdc-e2e-*` Starter cluster without a spending limit, prepares tdc-managed read-only/read-write/admin SQL users, creates connection strings, executes the HTTPS SQL API with all three access modes, creates one `tdc-e2e-branch-*` branch on that cluster, lists/describes/deletes the branch, updates the cluster, reads it again, deletes it, and verifies deletion. As TiDB Cloud API commands are implemented, their real live tests must be added to this same target.

## GitHub Actions

The repository has two CI workflows:

- `ci`: runs on pull requests and pushes to `main`. It downloads dependencies, checks `gofmt`, checks `go mod tidy`, runs `make test`, runs `make e2e`, and runs `make build`. It does not use TiDB Cloud credentials or live services.
- `live-e2e`: runs only when manually started with `workflow_dispatch`. It uses repository-level variables and secrets, configures the `live-e2e` profile, creates a temporary `tdc-live-e2e-*` tdc fs resource, runs `make live-e2e`, and attempts to delete that temporary fs resource in an always-run cleanup step.

Configure these repository variables for `live-e2e`:

```text
TDC_REGION_CODE=aws-us-east-1
```

Configure these repository secrets for `live-e2e`:

```text
TDC_PUBLIC_KEY=...
TDC_PRIVATE_KEY=...
```

The workflow stores generated `~/.tdc/` state under the runner temp directory and does not upload it as an artifact. The live suite creates and deletes real TiDB Cloud Starter and tdc fs resources scoped to unique `tdc-e2e-*` and `tdc-live-e2e-*` names. On GitHub-hosted Linux runners, the workflow installs `fuse3` before running the suite; if `/dev/fuse` is unavailable on the hosted runner, move live mount coverage to a FUSE-capable runner instead of weakening `make live-e2e`.

Clean build artifacts:

```bash
make clean
```

`make clean` removes `bin/` and `dist/`; both are ignored by git.

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
TDC_REGION_CODE=aws-us-east-1 \
TDC_PUBLIC_KEY="$TDC_PUBLIC_KEY" \
TDC_PRIVATE_KEY="$TDC_PRIVATE_KEY" \
bin/tdc configure --profile ci --non-interactive
```

You can also pass values as flags:

```bash
bin/tdc configure \
  --profile ci \
  --region-code aws-us-east-1 \
  --tdc-public-key "$TDC_PUBLIC_KEY" \
  --tdc-private-key "$TDC_PRIVATE_KEY" \
  --non-interactive
```

For CI/CD, prefer environment variables for secrets so private keys do not appear in shell history or process lists.

`tdc configure` prompts for:

- region code
- TiDB Cloud public key
- TiDB Cloud private key

The private key is not printed after entry. When stdin is a terminal, private key input is read without echo. Pressing Ctrl+C interrupts `tdc configure` and exits with code 130.

## Command Rules

`tdc` is designed for agents and scripts:

- Use long flags only. Short flags such as `-h` are rejected.
- Help is available as an explicit command, for example `tdc db help`.
- Successful structured command results render as JSON by default.
- `--output json` and `--output text` are supported output modes.
- `--query <jmespath-expression>` is applied after command execution and before rendering.
- Mutating control-plane commands support `--dry-run`.
- `--dry-run` loads the active profile and validates local config, credentials, provider, and region before reporting the planned mutation.
- Read-only commands reject `--dry-run`.
- Authenticated command failures use stable exit codes: `3` for authentication, `4` for authorization, and `5` for remote not found.
- Errors are rendered at the CLI boundary as:

```text
tdc [ERROR]: <actionable message>
```

Global flags:

- `--profile <name>`
- `--region <canonical-region-code>`
- `--debug`
- `--output <json|text>`
- `--query <jmespath-expression>`
- `--help`
- `--version`

## Local Operation Logs

tdc writes a local JSON Lines operation log by default:

```text
~/.tdc/logs/tdc.jsonl
```

The log is intended for local audit and debugging of agent/user activity. It records safe summaries such as command path, flag names, profile, region code, duration, exit code, tdc error code/category, remote service, HTTP method, HTTP status, and request id when available. It does not record flag values, SQL text, SQL results, file contents, raw request or response bodies, connection strings, local paths, tdc fs raw paths, API keys, DB passwords, or tdc fs API keys.

Disable operation logging for the current process:

```bash
TDC_LOGGING=off tdc db list-db-clusters
```

Disable it in local config:

```toml
[logging]
enabled = false
```

Supported `TDC_LOGGING` values are `on`, `true`, `1`, `yes`, `off`, `false`, `0`, and `no`. The environment variable takes precedence over `[logging].enabled`. Log rotation defaults to `10MB x 5` total files and can be tuned with `[logging].max_file_mb` and `[logging].max_files`.

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

- `--region-code <region-code>`, for example `aws-us-east-1` or `ali-ap-southeast-1`
- `--tdc-public-key <key>`
- `--tdc-private-key <key>`
- `--non-interactive`

### Update

```bash
tdc update --check
tdc update --check --fail-if-update-available
tdc update
tdc update --dry-run
tdc update --yes
tdc update --target-version v0.1.0 --yes
```

`tdc update --check` calls the GitHub Releases API for `github.com/tidbcloud/tdc`, matches the release artifact for the current OS/arch, and reports whether a newer release is available. It supports `--output json|text` and `--query`.

`update` only mutates official archive/script installs. It refuses local builds, unknown installs, and future package-manager installs with actionable errors. Use `--dry-run` to preview the selected artifact, checksum, and target path. Use `--yes` to replace the current binary on Unix-like platforms. On Windows, rerun `install.ps1` for the target version.

### Organization

```bash
tdc organization list-projects
tdc organization list-projects --page-size 10
tdc organization list-projects --page-token <next-page-token>
tdc organization list-projects --query 'projects[0].id'
tdc organization list-projects --output text
```

This command calls the TiDB Cloud IAM/account API with the active profile's Digest-auth API key pair and returns the projects visible to that profile.

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
tdc db delete-db-cluster --db-cluster-id <cluster-id>
tdc db delete-db-cluster --db-cluster-id <cluster-id> --dry-run
```

These commands call the TiDB Cloud Starter API with the active profile's Digest-auth API key pair. Create requires `--db-cluster-type starter` and a `--project-id`; discover project ids with `tdc organization list-projects`. Cluster JSON output uses stable snake_case fields such as `id`, `display_name`, and `next_page_token`.

Delete is non-interactive. Normal execution reads the remote cluster first, verifies it is a Starter cluster when the API returns plan metadata, and then deletes by cluster ID.

### DB Branch

```bash
tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev
tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev --dry-run
tdc db list-db-cluster-branches --db-cluster-id <cluster-id>
tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --page-size 10
tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --query 'branches[].id'
tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --output text
tdc db describe-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id>
tdc db describe-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --view FULL
tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id>
tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --dry-run
```

These commands call the TiDB Cloud Starter branch API with the active profile's Digest-auth API key pair. Create currently sends the API-backed `displayName` field through `--db-cluster-branch-name`.

Delete is non-interactive. Normal execution reads the remote branch first and then deletes by branch ID.

### DB SQL

```bash
tdc db create-db-sql-users --db-cluster-id <cluster-id>
tdc db create-db-sql-users --db-cluster-id <cluster-id> --dry-run
tdc db format-db-connection-string --db-cluster-id <cluster-id>
tdc db format-db-connection-string --db-cluster-id <cluster-id> --read-write --format mysql-uri
tdc db format-db-connection-string --db-cluster-id <cluster-id> --read-only --format env
tdc db format-db-connection-string --db-cluster-id <cluster-id> --admin --format jdbc
tdc db execute-sql-statement --db-cluster-id <cluster-id> --sql "select 1"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-write --sql "insert into t values (1)"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-only --sql "select * from t"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --admin --sql "show grants"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --transport https --sql "select 1"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --transport mysql --sql "select 1"
```

`create-db-sql-users` creates or repairs three stable tdc-managed SQL users for the cluster:

- `read_only`, backed by TiDB Cloud role `role_readonly`
- `read_write`, backed by TiDB Cloud role `role_readwrite`
- `admin`, backed by TiDB Cloud role `role_admin`

Generated DB SQL usernames and passwords are stored under:

```text
~/.tdc/db_users/<cluster-id>/credentials
```

Re-running `create-db-sql-users` is idempotent. It does not create a new user group when the tdc-managed users already exist. If local passwords are missing for verified tdc-managed remote users, it rotates those passwords through the SQL user API and writes the new local credentials.

`format-db-connection-string` and `execute-sql-statement` use read-write credentials by default. `--read-write`, `--read-only`, and `--admin` are mutually exclusive explicit selections. tdc never infers access mode from SQL text.

Connection string formats:

- `mysql-uri`
- `jdbc`
- `go-sql-driver`
- `sqlalchemy`
- `env`

`--format env` emits dotenv-compatible component variables directly, not JSON, so agents can compose framework-specific values without parsing URLs. Use `--env-include-database-url` to include a `DATABASE_URL`-style value.

`execute-sql-statement` executes exactly one SQL statement per invocation. HTTPS SQL is the default transport and uses `POST https://http-<cluster-host>/v1beta/sql` with Basic Auth from the prepared SQL credentials. `--transport mysql` is an explicit fallback that opens one MySQL connection, executes once, and closes it. Use `--output text` to render result sets as a terminal table. JSON remains the default output for agents and automation.

### tdc fs Control Plane

```bash
tdc fs check-file-system
tdc fs create-file-system --file-system-name workspace --dry-run
tdc fs create-file-system --file-system-name workspace
tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace --dry-run
tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace
```

`create-file-system` and `delete-file-system` are wired through the tdc fs control-plane client. Endpoint routing uses the hosted tdc fs region manifest and matches the active profile's canonical `region_code` against `tidb_cloud_native` entries. Users never provide a raw server URL. If the manifest does not include the profile placement, the command returns a clear unsupported-region error.

`create-file-system` provisions with the profile's TiDB Cloud API key pair in the HTTPS request body expected by the tdc fs backend. `delete-file-system` uses the stored `fs_api_key` as Bearer auth and also sends the TiDB Cloud key pair required for native tenant deletion. `--dry-run` validates config and shows a redacted request shape without printing credential values.

`create-file-system` stores returned resource metadata as flat `fs_*` keys in `~/.tdc/config` and stores the returned API key as `fs_api_key` under the active profile in `~/.tdc/credentials`. The API key is not printed in command output. `delete-file-system` clears the flat `fs_*` config and credential keys only after remote deletion succeeds.

`check-file-system` returns structured check status for local config, credentials, endpoint resolution, and remote service reachability. If `fs_api_key` has not been created yet, remote status is reported as a warning instead of making an unauthenticated `/v1/status` call. If the manifest does not support the configured placement, endpoint selection is reported as failed.

### tdc fs Data Plane

```bash
tdc fs copy-file --from-local ./README.md --to-remote /workspace/README.md
tdc fs copy-file --from-remote /workspace/README.md --to-local ./README.copy.md --create-parents
tdc fs copy-file --from-remote /workspace/README.md --to-remote /workspace/README.copy.md
tdc fs read-file --path /workspace/README.md
tdc fs read-file --path /workspace/README.md --offset 0 --length 1024
tdc fs copy-file --from-local ./tail.log --to-remote /workspace/app.log --append
tdc fs copy-file --from-remote /workspace/large.bin --to-local ./large.bin --resume
tdc fs copy-file --from-local ./large.bin --to-remote /workspace/large.bin --resume
tdc fs copy-file --from-local ./src-dir --to-remote /workspace/src-dir --recursive
tdc fs copy-file --from-remote /workspace/src-dir --to-local ./src-dir.copy --recursive
tdc fs copy-file --from-remote /workspace/src-dir --to-remote /workspace/src-dir.copy --recursive
tdc fs copy-file --from-local ./README.md --to-remote /workspace/layered.md --layer-id layer-1
printf 'hello\n' | tdc fs copy-file --from-stdin --to-remote /workspace/stdin.txt --tag source=stdin --description "stdin upload"
tdc fs copy-file --from-remote /workspace/stdin.txt --to-stdout
tdc fs list-files --path /workspace
tdc fs list-files --path /workspace --output text
tdc fs describe-file --path /workspace/README.md
tdc fs move-file --from-remote /workspace/README.copy.md --to-remote /workspace/archive/README.md
tdc fs delete-file --path /workspace/archive/README.md
tdc fs delete-file --path /workspace/archive --recursive
tdc fs create-directory --path /workspace/archive --mode 0755
tdc fs chmod-file --path /workspace/README.md --mode 0600
tdc fs create-symlink --target README.md --link-path /workspace/README.link
tdc fs create-hardlink --source-path /workspace/README.md --link-path /workspace/README.hard
tdc fs search-file-content --path /workspace --pattern "hello"
tdc fs search-file-content --path /workspace --pattern "hello" --layer-id layer-1
tdc fs find-files --path /workspace --file-name-pattern "*.md"
tdc fs find-files --path /workspace --file-name-pattern "*.md" --layer-id layer-1
```

The common data-plane commands also have Unix-style aliases. These aliases only replace the command name; flags stay long-only:

| Canonical | Alias |
| --- | --- |
| `tdc fs copy-file` | `tdc fs cp` |
| `tdc fs read-file` | `tdc fs cat` |
| `tdc fs list-files` | `tdc fs ls` |
| `tdc fs describe-file` | `tdc fs stat` |
| `tdc fs move-file` | `tdc fs mv` |
| `tdc fs delete-file` | `tdc fs rm` |
| `tdc fs create-directory` | `tdc fs mkdir` |
| `tdc fs chmod-file` | `tdc fs chmod` |
| `tdc fs create-symlink` | `tdc fs symlink` |
| `tdc fs create-hardlink` | `tdc fs hardlink` |
| `tdc fs search-file-content` | `tdc fs grep` |
| `tdc fs find-files` | `tdc fs find` |

Examples:

```bash
tdc fs cp --from-local ./README.md --to-remote /workspace/README.md
tdc fs cat --path /workspace/README.md
tdc fs ls --path /workspace --output text
tdc fs rm --path /workspace/archive --recursive
tdc fs grep --path /workspace --pattern "hello"
tdc fs find --path /workspace --file-name-pattern "*.md"
```

These commands use the active profile's stored `fs_api_key` as Bearer auth and call the tdc fs data-plane endpoint selected from the hosted region manifest. Run `tdc fs create-file-system` before using them, or configure the flat `fs_api_key` credential manually if the resource already exists.

`read-file` writes raw file bytes to stdout and does not wrap the response in JSON. Do not combine `read-file` with `--query`; queries require structured output. `--offset` and `--length` perform a byte-range read and must be provided together. Metadata and search commands return structured JSON by default and support `--output text` for terminal tables.

`copy-file` supports exactly one explicit source/target pair: `--from-local` with `--to-remote`, `--from-remote` with `--to-local`, or `--from-remote` with `--to-remote`. Remote and local targets are not overwritten unless `--overwrite` is provided. `--create-parents` only applies when copying from tdc fs to a local path. Large local-to-remote copies use multipart upload, preferring the Drive9-compatible V2 presign-batch protocol and falling back to V1 when the backend does not expose V2. Multipart uploads use bounded concurrency, retry once with a fresh presigned URL after an expired V2 part URL, and return `upload_mode` such as `multipart_v2`, `multipart_v1`, or `resume_v1` when that path is used. `--append` appends a local file to a remote file by using the tdc fs append plan when the backend supports it, then falls back to conditional rewrite for compatible non-S3-backed files. `--resume` resumes either an active local-to-remote multipart upload or a partial remote-to-local download. Local-to-remote resume requires an existing active upload for the same remote path; omit `--resume` for a fresh upload. `--recursive` copies directory contents into the target for local-to-remote, remote-to-local, and remote-to-remote copies. Local recursive copy rejects symlinks instead of silently following them. `--from-stdin` and `--to-stdout` provide raw stream copy paths for agents. Local-to-remote writes accept repeatable `--tag key=value` and `--description`; tags and descriptions are sent to the backend when the selected upload path supports them.

`chmod-file`, `create-symlink`, and `create-hardlink` map to the tdc fs POSIX-style metadata endpoints. tdc also stores client-side metadata for tdc-managed modes and symlink targets under `~/.tdc/fs_metadata`, so `describe-file` and FUSE can report those values even when remote stat metadata is sparse. Remote symlink creation is supported by the data-plane API; local overlay symlinks and tdc-created remote symlinks are readable through FUSE.

### tdc fs Layers

```bash
tdc fs create-layer --layer-id layer-1 --base-root-path /workspace --layer-name task --durability-mode restore-safe --tag task=auth
tdc fs list-layers
tdc fs list-layers --output text
tdc fs describe-layer --layer-id layer-1
tdc fs diff-layer --layer-id layer-1
tdc fs replay-layer --layer-id layer-1
tdc fs create-layer-entry --layer-id layer-1 --path /workspace/inline.txt --content "hello" --mode 0644
tdc fs upload-layer-file --layer-id layer-1 --from-local ./README.md --to-layer-path /workspace/README.md
tdc fs read-layer-file --layer-id layer-1 --path /workspace/README.md
tdc fs describe-layer-entry --layer-id layer-1 --path /workspace/README.md
tdc fs create-layer-checkpoint --layer-id layer-1 --checkpoint-id cp-1 --label before-commit
tdc fs describe-layer-checkpoint --checkpoint-id cp-1
tdc fs list-layer-events --layer-id layer-1
tdc fs rollback-layer --layer-id layer-1
tdc fs commit-layer --layer-id layer-1
```

Layer commands use the same tdc fs Bearer credentials as data-plane file commands. Mutating layer commands support `--dry-run`; read-only layer commands reject `--dry-run` like other read-only commands. `read-layer-file` writes raw file bytes to stdout and rejects `--query`.

`copy-file --layer-id` writes local-to-remote and remote-to-remote copy targets into a layer instead of mutating the base filesystem. It does not combine with `--append`, `--resume`, or `--recursive`. `search-file-content --layer-id` and `find-files --layer-id` search the layer overlay when the backend supports layer-aware search and find.

### tdc fs Pack And Unpack

```bash
tdc fs pack-file-system --local-root ~/.tdc/local/fs/demo --remote-root /workspace --mount-profile portable
tdc fs pack-file-system --mount-path ./workspace
tdc fs pack-file-system --local-root ~/.tdc/local/fs/demo --remote-root /workspace --mount-profile portable --archive-path /workspace/packs/demo.tar.gz
tdc fs unpack-file-system --local-root ~/.tdc/local/fs/demo --remote-root /workspace --mount-profile portable
tdc fs unpack-file-system --local-root ~/.tdc/local/fs/demo --archive-path /workspace/packs/demo.tar.gz
```

Pack archives preserve local overlay directories, regular files, symlinks, mode bits, and mtimes. The native archive format is `tdc.pack.v1`; unpack also accepts `drive9.pack.v1` archives. Archive manifests use `.tdc-pack-manifest.json`, and unpack also accepts Drive9's `.drive9-pack-manifest.json` for compatibility.

Mount profiles:

- `coding-agent`: default profile. Routes `.git`, dependency directories, caches, build output, and common temporary paths to the local overlay. It has no automatic pack paths.
- `portable`: packs and unpacks `/` by default and is intended for moving a local overlay between machines or sandbox sessions.
- `none`: disables local overlay profile behavior.

When `--archive-path` is omitted, tdc writes to `/.tdc/packs/<mount-profile>-<hash>.tar.gz` under the active tdc fs resource. `pack-file-system --mount-path` reads mount state from `~/.tdc/mounts/` and uses that mount's `local_root`, `remote_root`, mount profile, and pack paths. Unpack is staged and then installed, with path traversal and symlink-ancestor checks before local files are replaced.

### tdc Git Workspaces

```bash
tdc fs-git clone-git-workspace --repo-url https://github.com/pingcap/tidb.git --target-path ./workspace/tidb
tdc fs-git clone-git-workspace --repo-url https://github.com/pingcap/tidb.git --target-path ./workspace/tidb --blobless --hydrate sync
tdc fs-git hydrate-git-workspace --target-path ./workspace/tidb --timeout 30m
tdc fs-git restore-git-workspace --target-path ./workspace/tidb
tdc fs-git add-git-worktree --base-path ./workspace/tidb --worktree-path ./workspace/tidb-feature --branch-name feature-x
tdc fs-git remove-git-worktree --worktree-path ./workspace/tidb-feature --force
```

Git workspace commands are Drive9-style client workflows in tdc's explicit command naming. They require the target path to be inside a tdc fs mount with mount metadata. `clone-git-workspace` runs native `git clone --no-checkout`, registers a `/v1/git-workspaces` row, uploads the HEAD tree manifest with `/tree`, initializes the Git index with `git read-tree --reset`, and checkpoints lightweight `.git` state without the object database. `--blobless` adds `--filter=blob:none`; `--hydrate sync|background|off|auto` controls whether clean objects are prefetched after registration.

FUSE recognizes registered Git workspaces. Clean tracked files are served from the Git tree manifest and the local `.git` object database instead of ordinary `/v1/fs` file rows. Edits, chmod, symlink creation, hardlink copies, rename, and delete under a Git workspace are written to `/v1/git-workspaces/<id>/overlay`. The local `.git` directory stays in the mount profile's local overlay. `restore-git-workspace` downloads the registered lightweight Git state and object packs, restores the local `.git` layout, and unpacks objects. The FUSE runtime uses the same restore path automatically when a clean Git file is requested but local Git state is missing.

The lower-level API commands remain available for diagnostics and automation:

```bash
tdc fs-git create-git-workspace --root-path /workspace/repo --repo-url https://example.test/repo.git --mode fast
tdc fs-git list-git-workspaces
tdc fs-git describe-git-workspace --root-path /workspace/repo
tdc fs-git replace-git-tree --workspace-id <id> --commit-sha <sha> --node-json '{"path":"README.md","name":"README.md","kind":"file","mode":"100644","object_sha":"..."}'
tdc fs-git list-git-tree --workspace-id <id> --commit-sha <sha>
tdc fs-git upsert-git-state --workspace-id <id> --checkpoint-commit <sha> --storage-type inline --content state
tdc fs-git put-git-overlay-entry --workspace-id <id> --path README.md --operation upsert --resource-kind file --mode 100644 --content hello
tdc fs-git delete-git-workspace --workspace-id <id>
```

### tdc Vault

```bash
tdc fs-vault create-secret --secret-name db-prod --field DB_URL=mysql://example --field PASSWORD=@./password.txt
tdc fs-vault replace-secret --secret-path /n/vault/db-prod --from-directory ./secret-fields
tdc fs-vault read-secret --secret-name db-prod
tdc fs-vault read-secret --secret-name db-prod --field PASSWORD --format raw
tdc fs-vault read-secret --secret-name db-prod --field DB_URL --format env
tdc fs-vault list-secrets
tdc fs-vault delete-secret --secret-name db-prod
tdc fs-vault create-grant --agent-id deploy-agent --scope db-prod/DB_URL --permission read --ttl 10m
tdc fs-vault delete-grant --grant-id <grant-id> --reason rotated
tdc fs-vault create-token --agent-id deploy-agent --task-id deploy-123 --scope db-prod --ttl 10m
tdc fs-vault delete-token --token-id <token-id>
tdc fs-vault list-audit-events --secret-name db-prod --limit 20
tdc fs-vault run-with-secret --secret-path /n/vault/db-prod -- env
tdc fs-vault mount-vault --mount-path ./vault
tdc fs-vault mount-vault --mount-path ./vault --vault-token "$TDC_VAULT_TOKEN"
tdc fs-vault unmount-vault --mount-path ./vault
```

Vault commands use the active profile's tdc fs endpoint and stored `fs_api_key`; users do not configure a vault server URL. Mutating vault commands support `--dry-run`; read-only vault commands reject `--dry-run` like other read-only commands.

`create-secret` accepts repeatable `--field key=value` assignments. Use `key=@file` to read a field value from a local file, or `key=-` to read it from stdin. `replace-secret` replaces all fields from files in a directory named by the strict vault path form `/n/vault/<secret>`.

`read-secret` uses owner credentials by default. `--vault-token` or `TDC_VAULT_TOKEN` switches to delegated read endpoints. `--format raw` requires `--field` and prints the field bytes exactly. `--format env` emits dotenv-compatible `KEY=value` lines.

`create-grant` is the preferred delegated access flow. It mints a scoped token for an agent and returns both `token` and `grant_id`. `create-token` exists for legacy Drive9-compatible scoped tokens and returns `token` and `token_id`. `run-with-secret` reads one secret and injects its fields into a child process environment. It rejects field names outside `[A-Z_][A-Z0-9_]*`, rejects control bytes in values except tabs, and removes tdc credential environment variables from the child process.

`mount-vault` exposes readable secrets as a read-only FUSE filesystem: `<mount>/<secret>/<field>`. Owner mode uses the active profile's `fs_api_key`. Delegated mode uses `--vault-token` or `TDC_VAULT_TOKEN` and calls the delegated read endpoints. The default mode starts a background mount process and records state under `~/.tdc/mounts/`; `--foreground` keeps it attached to the current terminal. When a token is needed for the background process, tdc passes it through `TDC_VAULT_TOKEN` in the child environment instead of process arguments. `unmount-vault` stops the recorded mount process.

### tdc Journal

```bash
tdc fs-journal create-journal --journal-id jrn-demo --journal-kind agent --title "demo task" --actor agent:tdc --label env=dev
tdc fs-journal append-journal-entries --journal-id jrn-demo --entry-json '{"type":"task.started","summary":{"message":"hello"}}'
printf '%s\n' '{"type":"task.completed"}' | tdc fs-journal append-journal-entries --journal-id jrn-demo
tdc fs-journal read-journal-entries --journal-id jrn-demo --after-seq 0 --limit 100
tdc fs-journal search-journal-entries --entry-type task.started --label env=dev --include-entries
tdc fs-journal verify-journal --journal-id jrn-demo --output text
```

Journal commands use the active profile's tdc fs endpoint and stored `fs_api_key`. Users do not configure a journal server URL. Mutating journal commands support `--dry-run`; read-only journal commands reject `--dry-run`.

`create-journal` generates a journal id when `--journal-id` is omitted. `append-journal-entries` accepts repeatable `--entry-json` JSON objects or reads JSONL from stdin. Use `--json-array` when stdin contains a single JSON array. Every appended entry must have a `type`, either in the JSON object or through `--entry-type`. `--idempotency-key` is sent as the backend `Idempotency-Key` header; when omitted, tdc generates one.

`search-journal-entries` maps Drive9-style filters to the `/v1/journal-entries` query endpoint: `--entry-type`, `--status`, `--journal-kind`, `--actor`, repeatable `--subject`, repeatable `--label key=value`, `--since`, `--until`, `--cursor`, and `--include-entries`.

### tdc fs Mount Runtime

```bash
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --remote-path /projects/demo
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --driver fuse
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --driver webdav
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --foreground
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --mount-profile coding-agent
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --mount-profile portable
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --mount-profile portable --pack-path /
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --local-root ~/.tdc/local/fs/demo
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --unpack-archive-path /workspace/packs/demo.tar.gz
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --no-auto-unpack
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --read-cache-size-mb 256 --read-cache-max-file-mb 16
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --cache-dir ~/.tdc/cache/workspace --write-back-cache=false
tdc fs drain-file-system --mount-path ./workspace
tdc fs drain-file-system --mount-path ./workspace --timeout 30s
tdc fs unmount-file-system --mount-path ./workspace
tdc fs unmount-file-system --mount-path ./workspace --pack-archive-path /workspace/packs/demo.tar.gz
tdc fs unmount-file-system --mount-path ./workspace --no-auto-pack
tdc fs unmount-file-system --mount-path ./workspace --ignore-absent
```

Mount runtime aliases are `tdc fs mount` for `tdc fs mount-file-system`, `tdc fs drain` for `tdc fs drain-file-system`, and `tdc fs umount` for `tdc fs unmount-file-system`.

`mount-file-system` defaults to `--driver auto`, which prefers FUSE and falls back to WebDAV only when FUSE prerequisites are unavailable and WebDAV is supported. Use `--driver fuse` to require FUSE, or `--driver webdav` to force the compatibility bridge. The default background mode starts a foreground child process and records mount metadata under `~/.tdc/mounts/`; FUSE mounts also record a local control socket. `drain-file-system` connects to that socket and flushes dirty open handles plus pending write-back cache without unmounting. `unmount-file-system` reads the state and stops the mount process. `--foreground` keeps the mount runtime attached to the current terminal until interrupted.

FUSE mounts also record `mount_profile`, `local_root`, and `pack_paths` in mount state. When `--local-root` is omitted, tdc derives a stable local root under `~/.tdc/local/fs/<hash>` from the profile, fs resource, endpoint, remote root, and API key fingerprint. The `coding-agent` profile keeps VCS metadata, dependency directories, caches, build output, and common temporary paths in the local overlay. The `portable` profile defaults to pack path `/`, attempts to unpack the default archive on mount, and packs the local overlay back to tdc fs on unmount. Use `--no-auto-unpack` or `--no-auto-pack` to disable those automatic steps, or pass explicit archive paths when deterministic archive locations are required.

The FUSE driver uses `github.com/hanwen/go-fuse/v2` and the existing tdc fs data-plane API directly. It keeps a bounded in-memory read cache and, by default, persists pending writes under `~/.tdc/cache/mounts/<mount-hash>/` before uploading them on flush/release. The default cache identity includes the profile, fs resource name, tenant id, endpoint, remote root, mount path, and a fingerprint of the fs API key, so reusing the same local mount path for a different tdc fs resource gets a different cache directory. Pending write metadata is checked against that identity before recovery; mismatched pending writes are refused instead of uploaded to the wrong resource.

FUSE reads use `revision` and `resource_id` from the tdc fs stat API when those fields are available, so cached file bytes are reused only for the same known remote object version. Open writable file handles keep their base version and return a stale-file error if tdc can verify that the remote object changed or was deleted before flush. Rename retargets open handles, and deleting an open file marks the handle deleted so a later close does not recreate it. Git workspace paths add a synthetic tree layer backed by `/v1/git-workspaces` and local `.git` objects, with dirty changes persisted through Git overlay endpoints. On macOS FUSE requires macFUSE; on Linux it requires `/dev/fuse` plus `fusermount3` or `fusermount`.

The WebDAV driver starts a local loopback WebDAV bridge, supports WebDAV dead properties for client compatibility, and uses macOS `mount_webdav`/`umount`; Linux and Windows WebDAV mounts still return explicit unsupported-prerequisite errors. The CLI build remains cgo-free.

### All Commands

<details>
<summary>Show all commands</summary>

```text
tdc help
tdc --version
tdc configure
tdc configure --non-interactive
tdc update --check
tdc update
tdc organization list-projects
tdc db create-db-cluster
tdc db list-db-clusters
tdc db describe-db-cluster
tdc db update-db-cluster
tdc db delete-db-cluster
tdc db create-db-cluster-branch
tdc db list-db-cluster-branches
tdc db describe-db-cluster-branch
tdc db delete-db-cluster-branch
tdc db create-db-sql-users
tdc db format-db-connection-string
tdc db execute-sql-statement
tdc fs check-file-system
tdc fs create-file-system
tdc fs delete-file-system
tdc fs copy-file
tdc fs read-file
tdc fs list-files
tdc fs describe-file
tdc fs move-file
tdc fs delete-file
tdc fs create-directory
tdc fs chmod-file
tdc fs create-symlink
tdc fs create-hardlink
tdc fs search-file-content
tdc fs find-files
tdc fs create-layer
tdc fs list-layers
tdc fs describe-layer
tdc fs diff-layer
tdc fs replay-layer
tdc fs create-layer-entry
tdc fs upload-layer-file
tdc fs read-layer-file
tdc fs describe-layer-entry
tdc fs create-layer-checkpoint
tdc fs describe-layer-checkpoint
tdc fs list-layer-events
tdc fs rollback-layer
tdc fs commit-layer
tdc fs pack-file-system
tdc fs unpack-file-system
tdc fs mount-file-system
tdc fs drain-file-system
tdc fs unmount-file-system
tdc fs cp
tdc fs cat
tdc fs ls
tdc fs stat
tdc fs mv
tdc fs rm
tdc fs mkdir
tdc fs chmod
tdc fs symlink
tdc fs hardlink
tdc fs grep
tdc fs find
tdc fs mount
tdc fs drain
tdc fs umount
tdc fs-vault create-secret
tdc fs-vault replace-secret
tdc fs-vault read-secret
tdc fs-vault list-secrets
tdc fs-vault delete-secret
tdc fs-vault create-token
tdc fs-vault delete-token
tdc fs-vault create-grant
tdc fs-vault delete-grant
tdc fs-vault list-audit-events
tdc fs-vault run-with-secret
tdc fs-vault mount-vault
tdc fs-vault unmount-vault
tdc fs-journal create-journal
tdc fs-journal append-journal-entries
tdc fs-journal read-journal-entries
tdc fs-journal search-journal-entries
tdc fs-journal verify-journal
tdc fs-git clone-git-workspace
tdc fs-git hydrate-git-workspace
tdc fs-git restore-git-workspace
tdc fs-git add-git-worktree
tdc fs-git remove-git-worktree
tdc fs-git create-git-workspace
tdc fs-git list-git-workspaces
tdc fs-git describe-git-workspace
tdc fs-git delete-git-workspace
tdc fs-git replace-git-tree
tdc fs-git list-git-tree
tdc fs-git upsert-git-state
tdc fs-git describe-git-state
tdc fs-git put-git-object-pack
tdc fs-git list-git-object-packs
tdc fs-git describe-git-object-pack
tdc fs-git put-git-overlay-entry
tdc fs-git describe-git-overlay-entry
tdc fs-git list-git-overlay-entries
```

Help and version forms are also available at every command level:

```text
tdc <command> help
tdc <command> <subcommand> help
tdc <command> --version
tdc <command> <subcommand> --version
```

</details>

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

The credentials file is restricted to owner read/write permissions where the platform supports POSIX mode bits.

Minimum config shape:

```toml
[default]
region_code = "aws-us-east-1"
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

An explicit empty `--profile ""` is a usage error. Omitting `--profile` is not an error: the CLI uses `TDC_PROFILE` when it is set, otherwise it uses `default`. For shell scripts and CI jobs, prefer either a literal `--profile live-e2e` or an exported `TDC_PROFILE=live-e2e`.

Region selection order:

1. Non-empty `--region <canonical-region-code>`
2. `TDC_REGION_CODE=<canonical-region-code>`
3. `region_code` from the selected profile

`--region` is a command-scope override and has the highest placement priority. It does not persist to `~/.tdc/config` and does not change which profile or credential source is used. An explicit empty `--region ""` is a usage error.

Environment credentials are only a TiDB Cloud API key source. They do not change the selected local profile and do not create a local `[env]` profile. Generated tdc fs resource state is stored under `--profile`, `TDC_PROFILE`, or `default`.

Generated `tdc fs` resource credentials also live in `~/.tdc/credentials` as a flat key under the active profile:

```toml
[default]
fs_api_key = "..."
```

Generated non-secret `tdc fs` resource metadata lives in `~/.tdc/config` as flat keys under the active profile:

```toml
[default]
fs_resource_name = "workspace"
fs_tenant_id = "tenant-..."
fs_cloud_provider = "aws"
fs_region_code = "aws-us-east-1"
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

The DB SQL user credential path is not profile-scoped because TiDB Cloud cluster IDs are globally unique. The active profile only controls which TiDB Cloud API keys are used to prepare or repair those credentials. Do not add `[default.db_users."cluster-id".role]` sections to `~/.tdc/credentials`; the CLI rejects that legacy shape.

Do not configure TiDB Cloud API URLs, filesystem server URLs, metadata database URLs, or endpoint overrides in normal user config. Endpoint resolution is an internal responsibility derived from canonical `region_code`.

## API Auth And Endpoints

TiDB Cloud control-plane requests use HTTP Digest authentication with `tdc_public_key` as the digest username and `tdc_private_key` as the digest password. The private key is not sent as Basic Auth.

SQL HTTPS API execution uses the prepared DB SQL username and password as HTTP Basic Auth against `https://http-<cluster-host>/v1beta/sql`. Do not confuse these DB credentials with TiDB Cloud API keys.

Endpoint routing is internal:

- Starter API: `https://serverless.tidbapi.com`
- IAM/account API: `https://iam.tidbapi.com`
- tdc fs API: resolved from the hosted tdc fs region manifest, currently using `tidb_cloud_native` endpoint entries

Each control-plane command declares a permission requirement internally. Remote APIs remain the source of truth for the actual permission decision.

## Profile And Environment Lookup

Authenticated commands first select a local profile namespace:

1. Non-empty `--profile <name>`
2. `TDC_PROFILE=<name>`
3. `default`

Then tdc selects the TiDB Cloud API key source:

1. `TDC_PUBLIC_KEY` and `TDC_PRIVATE_KEY` when either is set
2. `tdc_public_key` and `tdc_private_key` from the selected local profile

Environment variables:

```bash
TDC_REGION_CODE=aws-us-east-1
TDC_PUBLIC_KEY=...
TDC_PRIVATE_KEY=...
TDC_LOGGING=off
```

When environment credentials are used, `TDC_PUBLIC_KEY` and `TDC_PRIVATE_KEY` are both required. `TDC_REGION_CODE` is optional when the selected local profile already has `region_code` or the command provides `--region`.

## Supported Cloud Placement

Users provide one canonical region code, never service URLs. The prefix before the first `-` selects the cloud provider: `aws` maps to AWS and `ali` maps to Alibaba Cloud.

| Region code | Cloud provider | Region label |
| --- | --- | --- |
| `aws-us-east-1` | AWS | N. Virginia |
| `aws-us-west-2` | AWS | Oregon |
| `aws-eu-central-1` | AWS | Frankfurt |
| `aws-ap-northeast-1` | AWS | Tokyo |
| `aws-ap-southeast-1` | AWS | Singapore |
| `ali-ap-southeast-1` | Alibaba Cloud | Singapore |

## Development Notes

Reference code under `ref/` is for context only. It is not imported, linked, tested against, or packaged as part of `tdc`.

Completed requirement specs are moved to:

```text
docs/spec/done/
```

Pending requirement specs remain in:

```text
docs/spec/
```

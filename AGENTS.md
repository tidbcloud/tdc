---
title: AGENTS.md - tdc development guide for AI coding agents
---

# Repository Overview

tdc is a Go command-line product for TiDB Cloud Starter. It is designed to be
agent-friendly, predictable, scriptable, and safe for automation.

Module: `github.com/tidbcloud/tdc`
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
- Starter DB branch lifecycle from
  `docs/spec/done/0007-starter-db-branch-lifecycle.md`
- Starter DB SQL access and query from
  `docs/spec/done/0008-starter-db-sql-access-and-query.md`
- tdc fs Unix-style command aliases from
  `docs/spec/done/0014-tdc-fs-unix-command-aliases.md`
- tdc fs control plane from
  `docs/spec/done/0009-tdc-fs-control-plane.md`
- tdc fs data plane from
  `docs/spec/done/0010-tdc-fs-data-plane.md`
- tdc fs mount runtime from
  `docs/spec/done/0011-tdc-fs-mount-runtime.md`
- tdc fs FUSE correctness and Drive9 parity extension from
  `docs/spec/done/0011-ext01-fuse-cache-and-open-handle-correctness.md`
- install and update distribution from
  `docs/spec/done/0012-install-and-update-distribution.md`
- `tdc configure`
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
- `tdc db create-db-sql-users`
- `tdc db format-db-connection-string`
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
- `tdc fs chmod-file`
- `tdc fs create-symlink`
- `tdc fs create-hardlink`
- `tdc fs search-file-content`
- `tdc fs find-files`
- `tdc fs create-layer`
- `tdc fs list-layers`
- `tdc fs describe-layer`
- `tdc fs diff-layer`
- `tdc fs replay-layer`
- `tdc fs create-layer-entry`
- `tdc fs upload-layer-file`
- `tdc fs read-layer-file`
- `tdc fs describe-layer-entry`
- `tdc fs create-layer-checkpoint`
- `tdc fs describe-layer-checkpoint`
- `tdc fs list-layer-events`
- `tdc fs rollback-layer`
- `tdc fs commit-layer`
- `tdc fs mount-file-system`
- `tdc fs drain-file-system`
- `tdc fs unmount-file-system`
- Unix-style `tdc fs` command aliases: `cp`, `cat`, `ls`, `stat`, `mv`, `rm`,
  `mkdir`, `chmod`, `symlink`, `hardlink`, `grep`, `find`, `mount`, `drain`,
  and `umount`
- `tdc vault create-secret`
- `tdc vault replace-secret`
- `tdc vault read-secret`
- `tdc vault list-secrets`
- `tdc vault delete-secret`
- `tdc vault create-token`
- `tdc vault delete-token`
- `tdc vault create-grant`
- `tdc vault delete-grant`
- `tdc vault list-audit-events`
- `tdc vault run-with-secret`
- `tdc vault mount-vault`
- `tdc vault unmount-vault`
- `tdc journal create-journal`
- `tdc journal append-journal-entries`
- `tdc journal read-journal-entries`
- `tdc journal search-journal-entries`
- `tdc journal verify-journal`
- help and version behavior at every command level
- structured JSON/text rendering and JMESPath `--query`
- `--dry-run` on mutating control-plane commands
- TiDB Cloud Digest-auth API client foundation and auth/authz error mapping
- flat `fs_*` config/credential storage for tdc fs control-plane resources
- default tdc fs FUSE mount runtime with version-aware read cache, local
  pending-write recovery, cache identity validation, local mount drain control
  socket, and WebDAV compatibility fallback
- tdc fs range reads, V2-first multipart upload with V1 fallback,
  local-to-remote upload resume, efficient append with fallback,
  remote-to-local download resume, FUSE dirty-range patch upload, and recursive
  local/remote copy
- tdc fs layer client and command coverage for create/list/describe/diff/replay,
  layer entry/object read-write, checkpoints, events, rollback, commit, and
  layer-aware copy/search/find flows
- tdc vault client and command coverage for secret create/read/replace/delete,
  delegated grants/tokens, audit listing, command environment injection, and a
  read-only vault mount filesystem view
- tdc journal client and command coverage for create/append/read/search/verify
  against the Drive9-compatible journal endpoints
- tdc fs pack/unpack workflows, mount-profile local overlays, portable
  auto-pack/auto-unpack, POSIX-style chmod/symlink/hardlink, stdin/stdout copy,
  tags, and descriptions
- tdc git workspace workflows for fast clone, hydrate, restore, linked worktree
  add/remove, low-level git workspace/tree/state/object-pack/overlay commands,
  and FUSE synthetic Git tree/overlay handling with restore of missing local
  Git state
- GoReleaser/GitHub Releases install and update workflow
- Makefile build/test/e2e workflow

There are no registered placeholder commands at the current stage. Implemented
mutating commands support `--dry-run` where their command contract declares
dry-run support.

## Reference Code

- `ref/tidbcloud-cli/` is the previous TiDB Cloud CLI implementation. Use it as
  a reference for TiDB Cloud concepts, profile handling, output helpers,
  telemetry, and API client patterns.
- `ref/drive9/` is the filesystem reference implementation. Use it as context
  for filesystem commands, mount behavior, and data-plane semantics. In tdc
  user-facing output, this domain is always called `tdc fs`.
- `ref/serverless-js/` is a reference for the HTTPS SQL API call shape.

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
make release-snapshot
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
that cluster. For Starter DB branches, the live suite creates, reads, lists,
and deletes only a `tdc-e2e-branch-*` branch on the cluster created by the same
test run. For Starter DB SQL access, the live suite prepares tdc-managed
read-only, read-write, and admin SQL users on the temporary cluster, verifies
connection string output, and executes the HTTPS SQL API with all three access
modes.
For tdc fs data-plane and mount runtime, the live suite creates uniquely named
remote paths, exercises real file create/read/list/copy/move/delete flows,
range reads, V2-first multipart upload, efficient append, upload resume,
remote-to-local resume, recursive local/remote copy, stdin/stdout copy,
tags/descriptions, chmod, symlink, hardlink, pack/unpack, real layer
create/read/diff/replay/checkpoint/events/commit flows, layer-aware copy/find,
vault create/read/replace/delete, delegated vault grant/token reads, vault
mount read on macOS/Linux FUSE hosts, journal create/append/read/search/verify,
low-level Git workspace API CRUD, mounts with the default FUSE driver when
platform prerequisites exist, verifies `tdc fs drain-file-system` against the
mounted runtime control socket, and also verifies explicit WebDAV fallback on
macOS when `mount_webdav` is available.
If the live profile has no `fs_api_key`, the suite creates a temporary tdc fs
resource named by `TDC_LIVE_FS_NAME` or `workspace`, stores the generated flat
`fs_*` metadata and `fs_api_key`, and deletes that auto-created resource when
the test process exits.
When a service command is implemented, add its real live verification to
`make live-e2e`; do not leave the target at profile, smoke-test-only, or
mock-only coverage.

For focused work, direct Go commands are also fine:

```bash
go test ./...
go test ./internal/config -run TestName
go build ./cmd/tdc
```

Build and release artifacts are ignored through `.gitignore`. Do not commit
binaries or GoReleaser `dist/` output.

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
internal/cli/               command wiring
internal/config/            profile loading and precedence rules
internal/config/configure/  interactive configure wizard
internal/config/fsresource/ flat tdc fs config key names
internal/config/region/     provider and region validation
internal/config/store/      TOML read/write, file modes, atomic writes
internal/db/                Starter DB cluster, branch, and SQL use cases
internal/db/connectionstring/ DB connection string formatters
internal/db/sqlaccess/      DB SQL user preparation logic
internal/db/sqlcred/        cluster-scoped DB SQL credential store
internal/db/sqlhttp/        HTTPS SQL API transport
internal/db/sqlmysql/       explicit MySQL fallback transport
internal/db/sqlresult/      SQL result model and decoding
internal/db/sqlsingle/      one-statement validation
internal/db/validate/       DB flag and request validation helpers
internal/dryrun/            shared dry-run result envelope
internal/fs/                tdc fs control-plane, data-plane, and mount use cases
internal/output/            structured JSON/text/raw rendering
internal/organization/      organization project command use cases
internal/query/             JMESPath query application
internal/secretinput/       no-echo secret input helper
internal/update/            GitHub Releases update checks and self-update logic
internal/version/           build version metadata
scripts/                    installer scripts
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
- `tdc fs` Unix-style aliases are command-name aliases only. They must keep the
  same long flags, output modes, auth, permissions, dry-run behavior, and command
  handlers as their canonical commands.
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
- Keep the global `--version` behavior intact at every command level. Do not
  add command-specific `--version <value>` flags; use names such as
  `--target-version` when a command needs a version input.
- `tdc cli check-update` and `tdc cli update` use GitHub Releases metadata and
  must not read or mutate `~/.tdc/`.
- `tdc cli update` may replace only tdc-owned archive/script installs. It must
  refuse local, unknown, Homebrew, Scoop, Winget, or other package-manager
  installs with actionable guidance.
- Installer scripts prefer upgrading the active `tdc`/`tdc.exe` found on
  `PATH` unless `--install-dir` or `TDC_INSTALL_DIR` overrides it. On
  macOS/Linux, no active binary means defaulting to `/usr/local/bin` and using
  `sudo` for directory creation or binary replacement when needed.
- Installer scripts must detect PATH shadowing, bootstrap `~/.tdc/config` only
  when missing, print DB and tdc fs region lists, and show clear next steps.
  They must never write `~/.tdc/credentials`.

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
- `tdc cli check-update`
- `tdc cli check-update --fail-if-update-available`
- `tdc cli update --dry-run`
- `tdc cli update --yes`
- `tdc cli update --target-version v0.1.0 --yes`
- `tdc organization list-projects`
- `tdc organization list-projects --query 'projects[0].id'`
- `tdc organization list-projects --output text`
- `tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id>`
- `tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id> --dry-run`
- `tdc db list-db-clusters`
- `tdc db list-db-clusters --query 'clusters[].id'`
- `tdc db describe-db-cluster --db-cluster-id <cluster-id>`
- `tdc db update-db-cluster --db-cluster-id <cluster-id> --db-cluster-name new-name`
- `tdc db update-db-cluster --db-cluster-id <cluster-id> --monthly-spending-limit-usd-cents 1000 --dry-run`
- `tdc db delete-db-cluster --db-cluster-id <cluster-id>`
- `tdc db delete-db-cluster --db-cluster-id <cluster-id> --dry-run`
- `tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev`
- `tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev --dry-run`
- `tdc db list-db-cluster-branches --db-cluster-id <cluster-id>`
- `tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --query 'branches[].id'`
- `tdc db list-db-cluster-branches --db-cluster-id <cluster-id> --output text`
- `tdc db describe-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id>`
- `tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id>`
- `tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --dry-run`
- `tdc db create-db-sql-users --db-cluster-id <cluster-id>`
- `tdc db create-db-sql-users --db-cluster-id <cluster-id> --dry-run`
- `tdc db format-db-connection-string --db-cluster-id <cluster-id>`
- `tdc db format-db-connection-string --db-cluster-id <cluster-id> --read-write --format mysql-uri`
- `tdc db format-db-connection-string --db-cluster-id <cluster-id> --read-only --format env`
- `tdc db format-db-connection-string --db-cluster-id <cluster-id> --admin --format jdbc`
- `tdc db execute-sql-statement --db-cluster-id <cluster-id> --sql "select 1"`
- `tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-write --sql "select 1"`
- `tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-only --sql "select 1"`
- `tdc db execute-sql-statement --db-cluster-id <cluster-id> --admin --sql "select 1"`
- `tdc db execute-sql-statement --db-cluster-id <cluster-id> --transport https --sql "select 1"`
- `tdc db execute-sql-statement --db-cluster-id <cluster-id> --transport mysql --sql "select 1"`
- `tdc fs create-file-system --file-system-name workspace`
- `tdc fs create-file-system --file-system-name workspace --dry-run`
- `tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace`
- `tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace --dry-run`
- `tdc fs check-file-system`
- `tdc fs copy-file --from-local ./README.md --to-remote /workspace/README.md`
- `tdc fs copy-file --from-remote /workspace/README.md --to-local ./README.copy.md --create-parents`
- `tdc fs copy-file --from-remote /workspace/README.md --to-remote /workspace/README.copy.md`
- `tdc fs read-file --path /workspace/README.md`
- `tdc fs read-file --path /workspace/README.md --offset 0 --length 1024`
- `tdc fs copy-file --from-local ./tail.log --to-remote /workspace/app.log --append`
- `tdc fs copy-file --from-remote /workspace/large.bin --to-local ./large.bin --resume`
- `tdc fs copy-file --from-local ./large.bin --to-remote /workspace/large.bin --resume`
- `tdc fs copy-file --from-local ./src-dir --to-remote /workspace/src-dir --recursive`
- `tdc fs copy-file --from-remote /workspace/src-dir --to-local ./src-dir.copy --recursive`
- `tdc fs copy-file --from-remote /workspace/src-dir --to-remote /workspace/src-dir.copy --recursive`
- `tdc fs copy-file --from-stdin --to-remote /workspace/stdin.txt --tag source=stdin --description "stdin upload"`
- `tdc fs copy-file --from-remote /workspace/stdin.txt --to-stdout`
- `tdc fs list-files --path /workspace`
- `tdc fs list-files --path /workspace --output text`
- `tdc fs describe-file --path /workspace/README.md`
- `tdc fs move-file --from-remote /workspace/README.copy.md --to-remote /workspace/archive/README.md`
- `tdc fs delete-file --path /workspace/archive/README.md`
- `tdc fs delete-file --path /workspace --recursive`
- `tdc fs create-directory --path /workspace/archive --mode 0755`
- `tdc fs chmod-file --path /workspace/README.md --mode 0600`
- `tdc fs create-symlink --target README.md --link-path /workspace/README.link`
- `tdc fs create-hardlink --source-path /workspace/README.md --link-path /workspace/README.hard`
- `tdc fs search-file-content --path /workspace --pattern "hello"`
- `tdc fs search-file-content --path /workspace --pattern "hello" --layer-id layer-1`
- `tdc fs find-files --path /workspace --file-name-pattern "*.md"`
- `tdc fs find-files --path /workspace --file-name-pattern "*.md" --layer-id layer-1`
- `tdc fs create-layer --layer-id layer-1 --base-root-path /workspace --layer-name task --durability-mode restore-safe --tag task=auth`
- `tdc fs list-layers`
- `tdc fs list-layers --output text`
- `tdc fs describe-layer --layer-id layer-1`
- `tdc fs diff-layer --layer-id layer-1`
- `tdc fs replay-layer --layer-id layer-1`
- `tdc fs create-layer-entry --layer-id layer-1 --path /workspace/inline.txt --content "hello" --mode 0644`
- `tdc fs upload-layer-file --layer-id layer-1 --from-local ./README.md --to-layer-path /workspace/README.md`
- `tdc fs copy-file --from-local ./README.md --to-remote /workspace/layered.md --layer-id layer-1`
- `tdc fs read-layer-file --layer-id layer-1 --path /workspace/README.md`
- `tdc fs describe-layer-entry --layer-id layer-1 --path /workspace/README.md`
- `tdc fs create-layer-checkpoint --layer-id layer-1 --checkpoint-id cp-1 --label before-commit`
- `tdc fs describe-layer-checkpoint --checkpoint-id cp-1`
- `tdc fs list-layer-events --layer-id layer-1`
- `tdc fs rollback-layer --layer-id layer-1`
- `tdc fs commit-layer --layer-id layer-1`
- `tdc fs pack-file-system --local-root ~/.tdc/local/fs/demo --remote-root /workspace --mount-profile portable`
- `tdc fs pack-file-system --mount-path ./workspace`
- `tdc fs unpack-file-system --local-root ~/.tdc/local/fs/demo --remote-root /workspace --mount-profile portable`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --driver fuse`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --driver webdav`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --mount-profile coding-agent`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --mount-profile portable --pack-path /`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --read-cache-size-mb 256 --read-cache-max-file-mb 16`
- `tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace --cache-dir ~/.tdc/cache/workspace --write-back-cache=false`
- `tdc fs drain-file-system --mount-path ./workspace`
- `tdc fs drain-file-system --mount-path ./workspace --timeout 30s`
- `tdc fs unmount-file-system --mount-path ./workspace`
- `tdc fs unmount-file-system --mount-path ./workspace --ignore-absent`
- `tdc vault create-secret --secret-name db-prod --field DB_URL=mysql://example --field PASSWORD=@./password.txt`
- `tdc vault replace-secret --secret-path /n/vault/db-prod --from-directory ./secret-fields`
- `tdc vault read-secret --secret-name db-prod`
- `tdc vault read-secret --secret-name db-prod --field PASSWORD --format raw`
- `tdc vault read-secret --secret-name db-prod --field DB_URL --format env`
- `tdc vault list-secrets`
- `tdc vault delete-secret --secret-name db-prod`
- `tdc vault create-grant --agent-id deploy-agent --scope db-prod/DB_URL --permission read --ttl 10m`
- `tdc vault delete-grant --grant-id <grant-id> --reason rotated`
- `tdc vault create-token --agent-id deploy-agent --task-id deploy-123 --scope db-prod --ttl 10m`
- `tdc vault delete-token --token-id <token-id>`
- `tdc vault list-audit-events --secret-name db-prod --limit 20`
- `tdc vault run-with-secret --secret-path /n/vault/db-prod -- env`
- `tdc vault mount-vault --mount-path ./vault`
- `tdc vault mount-vault --mount-path ./vault --vault-token "$TDC_VAULT_TOKEN"`
- `tdc vault unmount-vault --mount-path ./vault`
- `tdc journal create-journal --journal-id jrn-demo --journal-kind agent --title "demo task" --actor agent:tdc --label env=dev`
- `tdc journal append-journal-entries --journal-id jrn-demo --entry-json '{"type":"task.started"}'`
- `tdc journal read-journal-entries --journal-id jrn-demo --after-seq 0 --limit 100`
- `tdc journal search-journal-entries --entry-type task.started --label env=dev --include-entries`
- `tdc journal verify-journal --journal-id jrn-demo --output text`
- `tdc git clone-git-workspace --repo-url https://github.com/pingcap/tidb.git --target-path ./workspace/tidb`
- `tdc git clone-git-workspace --repo-url https://github.com/pingcap/tidb.git --target-path ./workspace/tidb --blobless --hydrate sync`
- `tdc git hydrate-git-workspace --target-path ./workspace/tidb --timeout 30m`
- `tdc git restore-git-workspace --target-path ./workspace/tidb`
- `tdc git add-git-worktree --base-path ./workspace/tidb --worktree-path ./workspace/tidb-feature --branch-name feature-x`
- `tdc git remove-git-worktree --worktree-path ./workspace/tidb-feature --force`
- `tdc git create-git-workspace --root-path /workspace/repo --repo-url https://example.test/repo.git --mode fast`
- `tdc git list-git-workspaces`
- `tdc git describe-git-workspace --root-path /workspace/repo`
- `tdc git replace-git-tree --workspace-id <id> --commit-sha <sha> --node-json '{"path":"README.md","name":"README.md","kind":"file","mode":"100644","object_sha":"..."}'`
- `tdc git list-git-tree --workspace-id <id> --commit-sha <sha>`
- `tdc git upsert-git-state --workspace-id <id> --checkpoint-commit <sha> --storage-type inline --content state`
- `tdc git put-git-overlay-entry --workspace-id <id> --path README.md --operation upsert --resource-kind file --mode 100644 --content hello`
- `tdc git delete-git-workspace --workspace-id <id>`

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
- `tdc db create-db-sql-users`
- `tdc db format-db-connection-string`
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
- `tdc fs chmod-file`
- `tdc fs create-symlink`
- `tdc fs create-hardlink`
- `tdc fs search-file-content`
- `tdc fs find-files`
- `tdc fs create-layer`
- `tdc fs list-layers`
- `tdc fs describe-layer`
- `tdc fs diff-layer`
- `tdc fs replay-layer`
- `tdc fs create-layer-entry`
- `tdc fs upload-layer-file`
- `tdc fs read-layer-file`
- `tdc fs describe-layer-entry`
- `tdc fs create-layer-checkpoint`
- `tdc fs describe-layer-checkpoint`
- `tdc fs list-layer-events`
- `tdc fs rollback-layer`
- `tdc fs commit-layer`
- `tdc fs pack-file-system`
- `tdc fs unpack-file-system`
- `tdc fs mount-file-system`
- `tdc fs drain-file-system`
- `tdc fs unmount-file-system`
- `tdc fs cp` aliases `tdc fs copy-file`
- `tdc fs cat` aliases `tdc fs read-file`
- `tdc fs ls` aliases `tdc fs list-files`
- `tdc fs stat` aliases `tdc fs describe-file`
- `tdc fs mv` aliases `tdc fs move-file`
- `tdc fs rm` aliases `tdc fs delete-file`
- `tdc fs mkdir` aliases `tdc fs create-directory`
- `tdc fs chmod` aliases `tdc fs chmod-file`
- `tdc fs symlink` aliases `tdc fs create-symlink`
- `tdc fs hardlink` aliases `tdc fs create-hardlink`
- `tdc fs grep` aliases `tdc fs search-file-content`
- `tdc fs find` aliases `tdc fs find-files`
- `tdc fs mount` aliases `tdc fs mount-file-system`
- `tdc fs drain` aliases `tdc fs drain-file-system`
- `tdc fs umount` aliases `tdc fs unmount-file-system`
- `tdc vault create-secret`
- `tdc vault replace-secret`
- `tdc vault read-secret`
- `tdc vault list-secrets`
- `tdc vault delete-secret`
- `tdc vault create-token`
- `tdc vault delete-token`
- `tdc vault create-grant`
- `tdc vault delete-grant`
- `tdc vault list-audit-events`
- `tdc vault run-with-secret`
- `tdc vault mount-vault`
- `tdc vault unmount-vault`
- `tdc journal create-journal`
- `tdc journal append-journal-entries`
- `tdc journal read-journal-entries`
- `tdc journal search-journal-entries`
- `tdc journal verify-journal`
- `tdc git clone-git-workspace`
- `tdc git hydrate-git-workspace`
- `tdc git restore-git-workspace`
- `tdc git add-git-worktree`
- `tdc git remove-git-worktree`
- `tdc git create-git-workspace`
- `tdc git list-git-workspaces`
- `tdc git describe-git-workspace`
- `tdc git delete-git-workspace`
- `tdc git replace-git-tree`
- `tdc git list-git-tree`
- `tdc git upsert-git-state`
- `tdc git describe-git-state`
- `tdc git put-git-object-pack`
- `tdc git list-git-object-packs`
- `tdc git describe-git-object-pack`
- `tdc git put-git-overlay-entry`
- `tdc git describe-git-overlay-entry`
- `tdc git list-git-overlay-entries`

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

Generated `tdc fs` resource credentials live in `~/.tdc/credentials` as flat
keys under the active profile:

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

DB SQL user credentials live outside the main credentials file:

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
APIs. SQL HTTPS API execution and tdc fs data-plane auth are separate
authentication schemes. SQL HTTPS API execution uses the prepared DB SQL
username/password as Basic Auth against
`https://http-<cluster-host>/v1beta/sql`; TiDB Cloud API keys must not be used
for SQL execution Basic Auth.

Use `internal/api/endpoints` for Starter, IAM/account, and fs endpoint
selection. Do not add service URLs to user config. The default Starter host is
`https://serverless.tidbapi.com`; the default IAM host is
`https://iam.tidbapi.com`. The tdc fs host is resolved from the hosted tdc fs
region manifest at
`https://drive9.ai/manifest/regions/drive9-regions.json`, matching the active
profile's cloud provider and region against `tidb_cloud_native` entries. If the
manifest does not contain the profile placement, return a clear unsupported
endpoint error; do not add a user-facing raw server URL flag or config key.

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
create-db-sql-users` owns those credentials and must be idempotent: it
creates or repairs the stable tdc-managed read-only, read-write, and admin
users for a cluster instead of creating a new group every time.

Generated `tdc fs` resource API keys also live in `~/.tdc/credentials`.
User-facing docs and commands must call these `tdc fs` API keys or resource
credentials, never reference implementation API keys. Filesystem data-plane
requests authenticate with `Authorization: Bearer <tdc-fs-api-key>` after the
resource is created. Native tdc fs provision/delete requests send the profile's
TiDB Cloud API key pair in the HTTPS JSON body expected by the backend; dry-run
and debug output must redact those credential values.

`tdc fs read-file --offset N --length M` is the byte-range read contract.
Both flags must be provided together. Large local-to-remote `copy-file` uses
V2-first multipart upload with V1 fallback, bounded concurrent part workers,
one fresh-presign retry for expired V2 part URLs, and complete-time tag
propagation in the internal fs client. `tdc fs copy-file --append` supports
only local-to-remote append and should use the tdc fs append plan when
available, with conditional rewrite only as a compatibility fallback. `tdc fs
copy-file --resume` supports both local-to-remote upload resume for an active
multipart upload and remote-to-local download resume for an existing partial
local file.
FUSE/writeback flushes should use dirty-range `PATCH /v1/fs/<path>` for
same-size writes with a known base revision and fall back to whole upload only
for unsupported backends, size-changing writes, or unknown base versions.
`tdc fs copy-file --recursive` copies directory contents into the target for
local-to-remote, remote-to-local, and remote-to-remote flows. Local recursive
copy must reject symlinks instead of silently following them.
`tdc fs copy-file --from-stdin` and `--to-stdout` are raw stream paths for
agents and must not wrap bytes in JSON. Local-to-remote writes accept
repeatable `--tag key=value` and `--description`; propagate those values
through multipart completion and ordinary write paths when the backend supports
them. `chmod-file`, `create-symlink`, and `create-hardlink` map to explicit tdc
fs data-plane endpoints. tdc persists client-side metadata for tdc-managed
modes and symlink targets under `~/.tdc/fs_metadata`, so `describe-file` and
FUSE can report those values when remote stat metadata is sparse. Local overlay
symlinks and tdc-created remote symlinks support readlink through FUSE.

`tdc fs` layer commands wrap the Drive9-compatible `/v1/layers`,
`/v1/layers/<layer_id>/*`, and `/v1/layer-checkpoints/<checkpoint_id>`
endpoints in tdc's two-level command style. Use `--layer-id`, not `--layer`.
`read-layer-file` returns raw bytes and rejects `--query`. The layer-aware copy
path, `copy-file --layer-id`, writes local-to-remote and remote-to-remote
targets into the layer. It must reject `--append`, `--resume`, and
`--recursive`.
`search-file-content --layer-id` and `find-files --layer-id` pass the layer
overlay selector to the backend.

`tdc vault` commands use the active profile's resolved tdc fs endpoint and
stored `fs_api_key`; do not add a raw vault server URL. Management and owner
read paths authenticate with `Authorization: Bearer <tdc-fs-api-key>`.
Delegated reads use `--vault-token` or `TDC_VAULT_TOKEN` with the same Bearer
scheme against `/v1/vault/read/*`. Mutating vault commands support `--dry-run`;
read-only vault commands reject it. `create-secret` accepts repeatable
`--field key=value`, `key=@file`, and `key=-` assignments. `replace-secret`
uses the strict path shape `/n/vault/<secret>` and reads fields from files in a
directory. `read-secret --format raw` requires `--field` and returns raw bytes;
`--format env` emits dotenv-compatible lines. `create-grant` is the preferred
delegated access command. `create-token` remains for Drive9-compatible legacy
scoped tokens. `run-with-secret` must reject environment keys outside
`[A-Z_][A-Z0-9_]*`, reject control bytes in values except tab, and scrub tdc
credential variables from the child process environment. `mount-vault` exposes
readable vault secrets as a read-only FUSE filesystem at
`<mount>/<secret>/<field>` and records shared mount state under
`~/.tdc/mounts/`. Owner mounts use `fs_api_key`; delegated mounts use
`--vault-token` or `TDC_VAULT_TOKEN`. For background delegated mounts, pass the
token through the child environment rather than process arguments.

`tdc journal` commands use the active profile's resolved tdc fs endpoint and
stored `fs_api_key`; do not add a raw journal server URL. They wrap
Drive9-compatible `/v1/journals`, `/v1/journals/<journal_id>/entries`,
`/v1/journals/<journal_id>/verify`, and `/v1/journal-entries`. Mutating
commands support `--dry-run`; read-only commands reject it.
`append-journal-entries` sends `--idempotency-key` as the `Idempotency-Key`
header, accepts repeatable `--entry-json` objects or JSONL stdin, and requires
every entry to have a type from the entry JSON or `--entry-type`.
`search-journal-entries` must preserve repeated `--label key=value` filters as
repeated `meta=key=value` query parameters.

`tdc fs mount-file-system` defaults to `--driver auto`, which prefers the FUSE
runtime and falls back to WebDAV only when FUSE prerequisites are unavailable
and WebDAV is supported. `--driver fuse` and `--driver webdav` are explicit
concrete selections. The FUSE runtime uses `github.com/hanwen/go-fuse/v2` and
the existing tdc fs data-plane client directly. It includes a bounded in-memory
read cache that validates known `revision`/`resource_id` metadata and a local
pending-write cache under `~/.tdc/cache/` that records mount identity before
recovery. FUSE mounts expose a local control socket recorded in mount state;
`tdc fs drain-file-system` must connect to that socket and flush dirty open
handles plus pending write-back cache without unmounting. WebDAV fallback
supports WebDAV dead properties for client compatibility. The implementation
must not import or depend on `ref/drive9`; copy or adapt concepts into
`internal/fs` instead. macOS FUSE requires macFUSE, Linux FUSE requires
`/dev/fuse` plus `fusermount3` or `fusermount`, and WebDAV fallback currently
uses macOS `mount_webdav`/`umount`. The CLI build must remain cgo-free.
Mount state stores `mount_profile`, `local_root`, and `pack_paths`. The default
mount profile is `coding-agent`; it routes `.git`, dependency directories,
caches, build output, and common temporary paths to the local overlay. The
`portable` profile defaults to pack path `/`, unpacks the default archive on
mount, and packs the local overlay on unmount unless the user passes
`--no-auto-unpack` or `--no-auto-pack`. Default local roots live under
`~/.tdc/local/fs/<hash>` and must not depend on the mount path alone.

`tdc git clone-git-workspace`, `hydrate-git-workspace`,
`restore-git-workspace`, `add-git-worktree`, and `remove-git-worktree` are
Drive9-style client workflows using tdc command names. They require a target
path inside a tdc fs mount with readable mount metadata. Fast clone/worktree
commands run native `git` with no checkout, register `/v1/git-workspaces`,
replace `/tree`, initialize the Git index, and checkpoint lightweight `.git`
state plus object packs. `restore-git-workspace` downloads registered Git
state and object packs, restores local `.git`, and runs
`git unpack-objects -r`. FUSE must serve registered clean Git files from the
Git tree manifest plus local `.git` object database, automatically restore
missing local Git state when possible, and persist dirty Git workspace changes
through `/v1/git-workspaces/<id>/overlay` rather than ordinary `/v1/fs` file
rows.

`tdc db format-db-connection-string` and `tdc db execute-sql-statement` use
read-write credentials by default. `--read-write`, `--read-only`, and `--admin`
must be mutually exclusive explicit selections. Do not add SQL-text
classification or an automatic access mode.

## Output And Errors

Use structured output contracts from the start.

- JSON is the default for successful structured control-plane commands.
- Data-plane commands may stream bytes or plain file listings when JSON would
  break expected filesystem usage.
- `--output json` and `--output text` are the initial output modes.
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

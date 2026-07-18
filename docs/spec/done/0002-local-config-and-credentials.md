# Local Config And Credentials

> **Current status:** This document records the original profile foundation. Filesystem credentials are no longer stored in the main `~/.tdc/credentials` file: the 1:N registry in `0016-profile-fs-resource-registry.md` stores each resource under `~/.tdc/fs_resources/<profile-key>/<resource-key>/`, and `0018-fs-token-auth-and-config-free-access.md` adds non-persistent environment/flag credentials. `0017-default-virtual-project-resolution.md` adds the configured `tidbx_virtual` `project_id`. DB SQL credentials remain in `~/.tdc/db_users/<cluster-id>/credentials`. Treat conflicting storage or resolution statements below as historical.

## Goal

Provide the local profile, config, and credentials foundation used by all
authenticated tdc commands.

## User-facing Commands

- `tdc configure [--profile <profile-name>]`

## Behavior

- `tdc configure` is interactive and may prompt for values.
- `tdc configure --non-interactive` is for automation and CI/CD. It reads
  values from flags first, then from `TDC_REGION_CODE`, `TDC_PUBLIC_KEY`, and
  `TDC_PRIVATE_KEY`; missing values fail without prompting.
- If `--profile` is omitted, configure the `default` profile.
- Store all local state under `~/.tdc/`.
- Store non-sensitive values in `~/.tdc/config`.
- Store profile-scoped sensitive values in `~/.tdc/credentials`.
- Store TiDB Cloud API keys and generated `tdc fs` resource API keys in
  `~/.tdc/credentials`.
- Generated DB SQL usernames and passwords are stored by
  `0008-starter-db-sql-access-and-query.md` under
  `~/.tdc/db_users/<cluster-id>/credentials`, not in the main
  `~/.tdc/credentials` profile file.
- Use TOML profile sections in both files.
- Create `~/.tdc/` and missing files as needed.
- Restrict credentials file permissions to owner read/write where supported.
- Prompt for canonical region code instead of any server URL.
- Supported MVP region codes are `aws-us-east-1`, `aws-us-west-2`,
  `aws-eu-central-1`, `aws-ap-northeast-1`, `aws-ap-southeast-1`, and
  `ali-ap-southeast-1`.
- Allow agents to bypass the interactive command by writing valid TOML directly;
  the CLI only needs to read the resulting files correctly.

## Inputs And Config

Minimum config shape:

```toml
# ~/.tdc/config
[default]
region_code = "aws-us-east-1"
fs_resource_name = "workspace"
fs_tenant_id = "tdcfs-tenant-id"
fs_cloud_provider = "aws"
fs_region_code = "aws-us-east-1"
```

Minimum credentials shape:

```toml
# ~/.tdc/credentials
[default]
tdc_public_key = "..."
tdc_private_key = "..."
fs_api_key = "..."
```

Local profile namespace lookup order:

1. If `--profile` is provided, use that profile.
2. If `TDC_PROFILE` is set, use that profile.
3. Otherwise use the `default` profile.

TiDB Cloud API key lookup order:

1. If either `TDC_PUBLIC_KEY` or `TDC_PRIVATE_KEY` is set, read both values from
   environment variables.
2. Otherwise read `tdc_public_key` and `tdc_private_key` from the selected local
   profile in `~/.tdc/credentials`.

Placement lookup is separate from credential lookup. A non-empty global
`--region <canonical-region-code>` overrides placement for the current command
only and has higher priority than `TDC_REGION_CODE` and profile `region_code`.
It does not persist config and does not change the selected profile or
credential source. `TDC_REGION_CODE` is also a command-scope placement override
and does not change where local state is read or written.

Environment credentials are a TiDB Cloud API key source only. They do not create
or select a local `[env]` profile. Generated tdc fs resource state is persisted
under the selected local profile: `--profile`, `TDC_PROFILE`, or `default`.

Do not store or accept server-url-like config keys, TiDB Cloud API endpoints, or
filesystem metadata database URLs as user configuration. Endpoint resolution is
implemented in the API client layer from canonical `region_code`.

## Output And Errors

- `tdc configure` may use human-readable prompts and completion messages.
- Do not print private keys after entry.
- Ctrl+C during interactive configure exits as an interruption instead of
  waiting for more input.
- Missing credentials for authenticated commands must identify the missing
  source and suggest running `tdc configure` or writing `~/.tdc/credentials`.
- Unsupported provider/region combinations must fail before any API request.

## After This Spec

Users can initialize the CLI once and then run authenticated commands from later
specs without repeating credentials:

```bash
tdc configure
tdc configure --profile stage
TDC_REGION_CODE=aws-us-east-1 TDC_PUBLIC_KEY=... TDC_PRIVATE_KEY=... tdc organization list-projects
TDC_PUBLIC_KEY=... TDC_PRIVATE_KEY=... tdc --region aws-ap-southeast-1 organization list-projects
```

Agents can create `~/.tdc/config` and `~/.tdc/credentials` directly using the
same TOML schema. The CLI remains responsible for validating the schema and
region matrix.

## Implementation Design

- `internal/config` exposes `Load(ctx, Options) (*Profile, error)` and hides all
  precedence rules.
- `internal/config/store` owns TOML read/write, directory creation, file modes,
  atomic writes, and testable home-directory injection.
- `internal/config/region` owns cloud provider enums, region enums, labels, and
  validation.
- `internal/config/fsresource` defines flat `fs_*` profile keys shared by fs
  control-plane, data-plane, and mount code.
- `internal/config/configure` owns the interactive wizard and calls the same
  store APIs used by tests.
- `internal/secretinput` reads private keys without echo when stdin is a
  terminal and falls back to ordinary line reads for non-terminal tests.

## API Call Chain

No remote API is called by this spec. `tdc configure` only writes local TOML
files. Authenticated API validation starts in
`0004-api-client-auth-and-region-routing.md`.

## Dependencies And Platform

- Add `github.com/pelletier/go-toml/v2` for TOML parsing and writing.
- Add `golang.org/x/term` for no-echo private-key input in `tdc configure`.
- Both dependencies are pure Go and do not require cgo.
- File permission enforcement must degrade with a clear warning on platforms
  where POSIX mode bits are not meaningful.

## Dependencies

- `0001-cli-foundation.md`.

## Acceptance Criteria

- Tests cover profile lookup with explicit `--profile`.
- Tests cover environment variable fallback.
- Tests cover default profile fallback.
- Tests cover AWS and Alibaba Cloud provider/region validation.
- Tests reject user-supplied server URL keys.
- Tests cover file creation and permissions for credentials.
- Tests cover storing and loading `tdc fs` resource API keys without printing
  them.
- Tests verify secrets are not printed by configure output.

## Out Of Scope

- OAuth login.
- Keyring storage.
- Migration from `~/.ticloud`.

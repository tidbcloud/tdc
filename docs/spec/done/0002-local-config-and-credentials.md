# Local Config And Credentials

## Goal

Provide the local profile, config, and credentials foundation used by all
authenticated tdc commands.

## User-facing Commands

- `tdc configure [--profile <profile-name>]`

## Behavior

- `tdc configure` is interactive and may prompt for values.
- `tdc configure --non-interactive` is for automation and CI/CD. It reads
  values from flags first, then from `TDC_CLOUD_PROVIDER`, `TDC_REGION_CODE`,
  `TDC_PUBLIC_KEY`, and `TDC_PRIVATE_KEY`; missing values fail without
  prompting.
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
- Prompt for cloud provider and region instead of any server URL.
- Supported MVP cloud providers are `aws` and `alibaba_cloud`.
- AWS regions are N. Virginia (`us-east-1`), Oregon (`us-west-2`), Frankfurt
  (`eu-central-1`), Tokyo (`ap-northeast-1`), and Singapore
  (`ap-southeast-1`).
- Alibaba Cloud supports Singapore (`ap-southeast-1`) only.
- Allow agents to bypass the interactive command by writing valid TOML directly;
  the CLI only needs to read the resulting files correctly.

## Inputs And Config

Minimum config shape:

```toml
# ~/.tdc/config
[default]
cloud_provider = "aws"
region_code = "us-east-1"
fs_resource_name = "workspace"
fs_tenant_id = "tdcfs-tenant-id"
fs_cloud_provider = "aws"
fs_region_code = "us-east-1"
```

Minimum credentials shape:

```toml
# ~/.tdc/credentials
[default]
tdc_public_key = "..."
tdc_private_key = "..."
fs_api_key = "..."
```

Lookup order:

1. If `--profile` is provided, read that profile from config and credentials.
2. If no profile is provided and environment variables are present, read
   `TDC_CLOUD_PROVIDER`, `TDC_REGION_CODE`, `TDC_PUBLIC_KEY`, and
   `TDC_PRIVATE_KEY`.
3. Otherwise read the `default` profile.

Do not store or accept server-url-like config keys, TiDB Cloud API endpoints, or
filesystem metadata database URLs as user configuration. Endpoint resolution is
implemented in the API client layer from `cloud_provider` and `region_code`.

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
TDC_CLOUD_PROVIDER=aws TDC_REGION_CODE=us-east-1 TDC_PUBLIC_KEY=... TDC_PRIVATE_KEY=... tdc organization list-projects
```

Agents can create `~/.tdc/config` and `~/.tdc/credentials` directly using the
same TOML schema. The CLI remains responsible for validating the schema and
provider/region matrix.

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

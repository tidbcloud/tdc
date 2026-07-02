# tdc fs Control Plane

## Goal

Provide lifecycle and health operations for tdc fs resources before exposing
file data-plane commands.

## User-facing Commands

Initial command set:

- `tdc fs create-file-system`
- `tdc fs delete-file-system`
- `tdc fs check-file-system`

## Behavior

- Use `tdc fs` as the product naming for the filesystem domain.
- Do not expose reference implementation names in user-facing command output.
- Mutating commands support `--dry-run`.
- Commands must not prompt.
- `check` validates local config, credentials, provider/region routing, endpoint
  resolution, authorization, and remote service reachability.
- Create provisions the remote fs resource according to the available fs API.
- Create stores the returned `tdc fs` resource API key in
  `~/.tdc/credentials`.
- Delete removes the selected fs resource using a non-interactive safety
  mechanism.
- No command accepts a user-provided server URL or filesystem metadata database
  URL.

## Inputs And Config

- Requires resolved credentials.
- Uses `cloud_provider`.
- Uses `region_code`.
- Resource identifiers must use explicit long flags.
- MVP supports one default tdc fs resource per profile. `--file-system-name`
  creates or validates the profile's `fs_resource_name`.
- Resource metadata is stored as flat `fs_*` keys under `[profile]` in
  `~/.tdc/config`: `fs_resource_name`, `fs_tenant_id`, `fs_cloud_provider`, and
  `fs_region_code`.
- Resource secrets are stored as flat `fs_*` keys under `[profile]` in
  `~/.tdc/credentials`: `fs_api_key`.

## Output And Errors

- JSON is the default output.
- `check` returns structured status for each checked dependency.
- Create/delete return resource or operation status.
- Create output must not print the raw `api_key` by default; it should report
  that credentials were stored.
- Errors must distinguish local configuration failures from remote fs service
  failures.
- Permission errors must name the required permission, such as
  `fs.volume.create`.

## After This Spec

Users can provision and validate `tdc fs` resources in the active profile's
cloud provider and region:

```bash
tdc fs check-file-system
tdc fs create-file-system --file-system-name workspace --dry-run
tdc fs create-file-system --file-system-name workspace
tdc fs delete-file-system --file-system-name workspace --confirm-file-system-name workspace
```

The user chooses provider and region during `tdc configure`; the CLI resolves
all fs endpoints internally.

## Implementation Design

- `internal/cli/fs` registers `tdc fs` commands and keeps all filesystem actions
  at the second command level.
- `internal/fs/control` owns create/delete/check use cases and authorization
  requirements.
- `internal/api/fs` contains fs control-plane HTTP methods and response models.
- `internal/fs/fscred` stores and loads the profile-level `fs_api_key` in
  `~/.tdc/credentials`.
- `internal/api/endpoints` provides fs endpoint resolution from
  `cloud_provider + region_code`.
- `internal/fs/status` defines the structured check response with local config,
  credential, permission, endpoint, and service health entries.
- Delete safety is implemented through an explicit long flag, initially
  `--confirm-file-system-name <name>`.

## API Call Chain

Confirmed from the reference filesystem protocol, but still requiring a hosted
tdc fs endpoint resolver before product implementation:

- `GET /v1/status` checks service reachability and reports tenant/server
  capabilities.
- `POST /v1/provision` provisions or initializes a tenant/resource.
- `DELETE /v1/tenant` deletes the tenant/resource.
- Successful provision responses are expected to include `tenant_id`,
  `api_key`, and `status`.

Command mapping:

- `tdc fs check-file-system`
  1. Validate local profile, credentials, provider, and region.
  2. Resolve the tdc fs base URL from `cloud_provider + region_code`.
  3. Call `GET /v1/status`.
  4. Report local config, endpoint resolution, auth, and remote status.
- `tdc fs create-file-system`
  1. Validate `--file-system-name` and dry-run behavior.
  2. Resolve the tdc fs base URL.
  3. Call `POST /v1/provision` with the confirmed tdc fs request body once the
     hosted API contract is finalized.
  4. Store `fs_resource_name`, `fs_tenant_id`, `fs_cloud_provider`, and
     `fs_region_code` in `[profile]` of `~/.tdc/config`.
  5. Store `fs_api_key` in `[profile]` of `~/.tdc/credentials`.
- `tdc fs delete-file-system`
  1. Validate `--confirm-file-system-name`.
  2. Resolve the tdc fs base URL.
  3. Load the stored resource API key and call `DELETE /v1/tenant` or the
     confirmed tdc fs delete-file-system endpoint with
     `Authorization: Bearer <api-key>`.
  4. Remove local `fs_*` metadata and secret only after remote deletion
     succeeds, unless an explicit local-forget command is added later.

API gap:

- The reference protocol confirms endpoint shapes, but tdc still needs a
  product-owned provider/region-to-host mapping and final provision/delete
  request schema. Until that contract is confirmed, implementation can build the
  client shape behind mocks but must not claim live create/delete support.
- The provision response's `api_key` is sensitive and must never be logged,
  emitted to telemetry, or shown in default command output.

## Dependencies And Platform

- No new third-party dependency beyond specs `0001` through `0004`.
- Uses the shared authenticated HTTP client.
- No cgo is required.
- Platform-neutral.

## Dependencies

- `0004-api-client-auth-and-region-routing.md`.
- `0003-output-error-query-dry-run.md`.

## Acceptance Criteria

- Mock API tests cover create/delete/check.
- Tests cover endpoint resolution from AWS and Alibaba Cloud provider/region
  values.
- Tests reject user-supplied server URL flags or config keys.
- Tests cover dry-run for create and delete.
- Tests cover storing fs `api_key` in credentials and redacting it from output,
  logs, and telemetry.
- Tests verify user-facing output says `tdc fs`, not the reference name.

## Out Of Scope

- File upload, download, listing, and mutation.
- Local mount lifecycle.

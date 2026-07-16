# Organization Project Context

## Goal

Expose the TiDB Cloud project context available through the confirmed IAM API so
agents and users can select a project before creating or managing resources.

## User-facing Commands

Initial command set:

- `tdc organization list-projects`

Do not add `tdc organization list` or `tdc organization describe` for the MVP:
the local API refs do not include organization list/describe endpoints.
Additional organization subcommands may be added only when backed by confirmed
IAM API capability and kept within the two-level command rule.

## Behavior

- Use authenticated TiDB Cloud IAM/account APIs.
- Return project identifiers, names, organization IDs, cluster counts, user
  counts, create timestamps, and available metadata that is safe to expose.
- Support `--profile`, `--output`, and `--query`.
- Support optional `--page-size` and `--page-token` for API pagination.
- Do not prompt.

## Inputs And Config

- Requires resolved credentials.
- Uses IAM endpoint routing from
  `0004-api-client-auth-and-region-routing.md`.

## Output And Errors

- JSON is the default output.
- If the active credentials cannot access projects, return an actionable
  permission error.
- Do not include credentials or tokens in output.

## After This Spec

Users can verify which projects their active profile can access before creating
DB or fs resources:

```bash
tdc organization list-projects
tdc organization list-projects --page-size 10
tdc organization list-projects --page-token <next-page-token>
tdc organization list-projects --query 'projects[0].id'
tdc organization list-projects --output text
```

This is the first practical authentication and authorization validation path for
new users after `tdc configure`.

## Implementation Design

- `internal/cli` defines Cobra commands and translates flags into service
  requests.
- `internal/organization` owns command use cases and applies the
  `organization.project.read` authorization requirement.
- `internal/api/iam` contains IAM/account HTTP methods and response models.
- `internal/output` renders project list responses.
- Keep project/account data models separate from DB and fs models so later IAM
  expansion does not leak into service packages.

## API Call Chain

Confirmed API:

1. Load resolved profile and credentials from `~/.tdc/config` and
   `~/.tdc/credentials`.
2. Build the TiDB Cloud IAM/account client with HTTP Digest auth.
3. Call `GET /v1beta1/projects` with optional `pageSize` and `pageToken`.
4. Decode `ApiListProjectsRsp`:
   - `projects[]`
   - `nextPageToken`
5. Normalize the CLI output field to `next_page_token`.
6. Render JSON by default; apply `--query` after decoding.

The confirmed project object fields are `id`, `name`, `type`, `org_id`,
`cluster_count`, `user_count`, `create_timestamp`, and `aws_cmek_enabled`.
`type` is `tidbx` for a regular project and `tidbx_virtual` for a virtual
project; `tidbx_virtual` is a value, not a separate boolean field.

## Dependencies And Platform

- No new third-party dependency beyond specs `0001` through `0004`.
- Uses the shared authenticated HTTP client.
- No cgo is required.
- Platform-neutral.

## Dependencies

- `0004-api-client-auth-and-region-routing.md`.

## Acceptance Criteria

- Mock API tests cover successful project list output and pagination.
- Tests cover permission-denied and unauthenticated errors.
- Tests cover `--query` extracting a single project field.
- `make live-e2e` covers the real `tdc organization list-projects` command
  through JSON, query, and text output.

## Out Of Scope

- Organization list/describe/create/delete until confirmed IAM endpoints exist.
- User, role, or invitation management.

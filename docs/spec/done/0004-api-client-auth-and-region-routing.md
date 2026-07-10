# API Client Auth, Authorization, And Region Routing

## Goal

Create the authenticated and authorization-aware client foundation for TiDB
Cloud Starter APIs, IAM-backed account APIs, and tdc fs APIs.

## User-facing Commands

No standalone command is required. This spec supports:

- `tdc db <subcommand>`
- `tdc fs <subcommand>`
- `tdc organization <subcommand>`

## Behavior

- Authenticate with TiDB Cloud public and private API keys.
- Build API clients from resolved profile/config/environment values.
- Route Starter DB requests by configured region.
- Route tdc fs requests by configured cloud provider and region.
- Route IAM-backed account requests to the TiDB Cloud IAM endpoint.
- Never ask users for server URLs, API endpoints, or filesystem metadata
  database URLs.
- Maintain an internal endpoint resolver keyed by `cloud_provider` and
  `region_code`.
- Respect `--debug` by enabling diagnostic logs without exposing secrets.
- Support request timeouts and cancellation.

## Authorization Model

- Authentication proves the API key pair is valid.
- Authorization proves the API key pair is allowed to perform the requested
  action on the requested resource.
- Every command declares one permission requirement before execution:
  - `organization.project.read`
  - `starter.cluster.read`
  - `starter.cluster.create`
  - `starter.cluster.update`
  - `starter.cluster.delete`
  - `starter.branch.read`
  - `starter.branch.create`
  - `starter.branch.delete`
  - `starter.sql_user.read`
  - `starter.sql_user.create`
  - `starter.sql_user.update`
  - `starter.sql.execute`
  - `fs.volume.read`
  - `fs.volume.create`
  - `fs.volume.delete`
  - `fs.file.read`
  - `fs.file.write`
  - `fs.mount`
- The CLI performs local preflight only for missing credentials, malformed
  credentials, unsupported provider/region, and command-to-permission mapping.
- The remote TiDB Cloud and tdc fs APIs remain the source of truth for actual
  permission decisions.
- If an API supports a non-mutating permission or capability check, mutating
  commands may call it before the mutation. If no such API exists, rely on the
  mutating API response and map permission failures consistently.
- TiDB Cloud control-plane calls use HTTP Digest authentication with
  `tdc_public_key` as the digest username and `tdc_private_key` as the digest
  password. The private key must not be sent as Basic Auth for these APIs.
- SQL execution in `0008-starter-db-sql-access-and-query.md` is separate: it
  uses Basic Auth with generated database SQL usernames and passwords against
  the HTTPS SQL API endpoint.
- tdc fs data-plane and mount calls are also separate: after a resource is
  created, they use the stored `tdc fs` resource API key as
  `Authorization: Bearer <api-key>`.

## Inputs And Config

Inputs come from the resolution behavior in
`0002-local-config-and-credentials.md`.

Required values for authenticated requests:

- `tdc_public_key`
- `tdc_private_key`
- canonical `region_code` when the target API is region-scoped

Canonical region matrix:

| Region code | Cloud provider | Native region |
| --- | --- | --- |
| `aws-us-east-1` | `aws` | `us-east-1` |
| `aws-us-west-2` | `aws` | `us-west-2` |
| `aws-eu-central-1` | `aws` | `eu-central-1` |
| `aws-ap-northeast-1` | `aws` | `ap-northeast-1` |
| `aws-ap-southeast-1` | `aws` | `ap-southeast-1` |
| `ali-ap-southeast-1` | `alibaba_cloud` | `ap-southeast-1` |

Service endpoint overrides are allowed only for tests or developer staging
builds. They must not be part of the normal user configuration path.

## Output And Errors

- Missing local credentials:

```text
tdc [ERROR]: authentication required: missing tdc_public_key and tdc_private_key for profile "default". Run `tdc configure` or set TDC_PUBLIC_KEY and TDC_PRIVATE_KEY.
```

- Invalid or rejected API keys:

```text
tdc [ERROR]: authentication failed: TiDB Cloud rejected the API key pair for profile "default". Check ~/.tdc/credentials or create a new API key.
```

- Permission denied:

```text
tdc [ERROR]: permission denied: profile "default" is not allowed to create Starter clusters in aws/us-east-1. Ask an organization admin for starter.cluster.create permission or use another profile.
```

- Missing region errors must name the expected key: `region_code` or
  `TDC_REGION_CODE`.
- Unsupported canonical region errors must show the valid region code list.
- Network and API errors must preserve machine-readable error codes internally
  for telemetry and tests.
- Initial exit-code mapping:
  - `1`: unknown runtime or remote API error
  - `2`: local usage, validation, or config error
  - `3`: authentication error
  - `4`: authorization or permission error
  - `5`: remote resource not found

## After This Spec

Commands from later specs can make authenticated calls and report permission
failures consistently:

```bash
tdc organization list-projects
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --dry-run
tdc fs check-file-system
```

Users never need to know service base URLs. They configure one canonical region
code, and the client resolver chooses the correct TiDB Cloud and tdc fs
endpoints.

## Implementation Design

- `internal/api` defines shared client options, request execution, response
  decoding, retry policy, and error mapping.
- `internal/api/transport` implements HTTP Digest authentication for TiDB Cloud
  control-plane APIs and redacts credentials from logs.
- `internal/api/endpoints` maps the parsed provider and native region from
  canonical `region_code` to Starter, IAM/account, and fs base URLs.
- `internal/auth` validates resolved credential presence and constructs the
  authenticated transport.
- `internal/authz` defines permission constants, command requirements, and
  helpers for formatting permission-denied errors.
- `internal/api/starter`, `internal/api/iam`, and `internal/api/fs` provide
  service-specific clients on top of the shared transport.
- API packages return typed errors; CLI packages decide how to render them.

## API Call Chain

TiDB Cloud control-plane API contract sources:

- Starter cluster and branch endpoints are present in the local generated
  Starter OpenAPI refs.
- IAM/account project and SQL user endpoints are present in
  `ref/tidbcloud-cli/pkg/tidbcloud/v1beta1/iam.swagger.json` and generated
  IAM clients, but are not currently confirmed in public product docs. Treat
  them as reference-derived APIs that tdc should try first. Implementation
  should keep these calls isolated behind `internal/api/iam`, cover them with
  mock tests, and surface live 404/405/501 responses as an API-gap error rather
  than silently falling back to an unsafe behavior.

- Base host: `https://serverless.tidbapi.com`.
- Auth: HTTP Digest, equivalent to `curl --digest --user
  '<public-key>:<private-key>'`.
- Starter cluster APIs:
  - `GET /v1beta1/clusters`
  - `POST /v1beta1/clusters`
  - `GET /v1beta1/clusters/{clusterId}`
  - `DELETE /v1beta1/clusters/{clusterId}`
  - `PATCH /v1beta1/clusters/{cluster.clusterId}`
  - `GET /v1beta1/regions`
  - `GET /v1beta1/regions:listCloudProviders`
- Starter branch APIs:
  - `GET /v1beta1/clusters/{clusterId}/branches`
  - `POST /v1beta1/clusters/{clusterId}/branches`
  - `GET /v1beta1/clusters/{clusterId}/branches/{branchId}`
  - `DELETE /v1beta1/clusters/{clusterId}/branches/{branchId}`
  - `POST /v1beta1/clusters/{clusterId}/branches/{branchId}:reset`
- IAM/account APIs found in the local reference and allowed for MVP trial use:
  - `GET /v1beta1/projects`
  - `GET /v1beta1/clusters/{clusterId}/sqlUsers`
  - `POST /v1beta1/clusters/{clusterId}/sqlUsers`
  - `GET /v1beta1/clusters/{clusterId}/sqlUsers/{userName}`
  - `PATCH /v1beta1/clusters/{clusterId}/sqlUsers/{userName}`
  - `DELETE /v1beta1/clusters/{clusterId}/sqlUsers/{userName}`

No confirmed organization list/describe endpoint exists in the local refs.
Specs must not expose `tdc organization list` or `tdc organization describe`
until an API contract is confirmed.

tdc fs endpoint routing depends on the hosted tdc fs product contract. The
filesystem protocol endpoints are documented in the fs specs, but the
provider/region-to-host resolver must be supplied by product configuration or a
confirmed service discovery API before implementation.

tdc fs resource authentication is reference-derived from the filesystem
protocol: control-plane provisioning returns an `api_key`, and subsequent fs
requests send it as `Authorization: Bearer <api-key>`. tdc stores that key in
`~/.tdc/credentials` as `fs_api_key` under the active profile.

## Dependencies And Platform

- Use Go standard library `net/http`, `context`, and `encoding/json`.
- Add `github.com/icholy/digest` for HTTP Digest authentication unless the
  project implements an equivalent tested transport internally.
- No cgo is required.
- Endpoint resolution is table-driven and platform-neutral.
- Do not depend on anything under `ref/`.

## Dependencies

- `0002-local-config-and-credentials.md`.
- `0003-output-error-query-dry-run.md`.

## Acceptance Criteria

- Tests use mock HTTP servers and do not require live TiDB Cloud credentials.
- Tests cover API key signing or request authentication construction.
- Tests cover region-derived endpoint selection.
- Tests cover provider/region-derived endpoint selection for AWS and Alibaba
  Cloud.
- Tests cover unsupported provider/region validation.
- Tests cover missing credentials, invalid credentials, and permission denied
  as distinct error categories and exit codes.
- Tests cover secret redaction in debug/error paths.

## Out Of Scope

- OAuth authentication.
- API pagination policy beyond what individual command specs require.
- Live API smoke tests; those are covered by the final smoke-test spec.

# Starter DB Cluster Lifecycle

## Goal

Provide Starter cluster lifecycle management under `tdc db`.

## User-facing Commands

Initial command set:

- `tdc db create-db-cluster`
- `tdc db list-db-clusters`
- `tdc db describe-db-cluster`
- `tdc db update-db-cluster`
- `tdc db delete-db-cluster`

Primary create shape:

```bash
tdc db create-db-cluster --db-cluster-name <name> --db-cluster-type starter
tdc db create-db-cluster --db-cluster-name <name> --db-cluster-type starter --wait-until-active
```

## Behavior

- `tdc db` manages TiDB Cloud Starter database clusters.
- Require `--db-cluster-type starter` for create to preserve future tier
  compatibility.
- Use long flags only.
- Mutating commands support `--dry-run`.
- Commands must not prompt.
- Create returns after the API accepts the asynchronous request by default.
  Optional `--wait-until-active` polls the created cluster until it reaches
  `ACTIVE`, for up to 12 minutes, and returns the final cluster representation.
- Waiting never deletes the created resource. Timeout, cancellation, polling
  failure, or a terminal `DELETING`, `DELETED`, or `INACTIVE` state returns an
  actionable error that includes the cluster ID and an inspection command.
- Delete must be non-interactive. It reads the remote cluster first, validates
  Starter-only behavior when plan metadata is available, and then deletes by
  cluster ID; it must never prompt.

## Inputs And Config

- Requires credentials and region routing.
- Common identifiers should use explicit names such as `--db-cluster-id` and
  `--db-cluster-name`.
- Project resolution follows `0017-default-virtual-project-resolution.md`:
  explicit `--project-id` overrides the profile's configured `project_id`.
- Optional create parameters should map directly to available Starter API
  fields. Do not invent unsupported fields.

## Output And Errors

- JSON is the default output.
- CLI output uses stable snake_case field names such as `id`, `display_name`,
  `next_page_token`, and `total_size`; the API client handles remote camelCase
  fields internally.
- Create and update return the remote resource representation or operation
  status returned by the API.
- Create with `--wait-until-active` returns a cluster whose state is `ACTIVE`.
  If waiting fails after creation, the error states that the cluster remains
  allocated and includes its ID.
- Delete returns a structured confirmation or operation status.
- Errors should distinguish validation failures, authentication failures,
  permission failures, not found, quota/capacity issues, and backend API errors.
- Permission errors must name the required permission, such as
  `starter.cluster.create`, without exposing credentials.

## After This Spec

Users can create and manage Starter clusters in the configured cloud provider
and region:

```bash
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --dry-run
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --wait-until-active
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --project-id <project-id>
tdc db list-db-clusters --query 'clusters[].id'
tdc db describe-db-cluster --db-cluster-id <id>
tdc db delete-db-cluster --db-cluster-id <id>
```

No command asks for a server URL. The active profile's `cloud_provider` and
`region_code` decide the target Starter API endpoint.

## Implementation Design

- `internal/cli` owns `tdc db` command registration and translates flags into
  service requests.
- `internal/db` owns cluster lifecycle use cases and declares authorization
  requirements for read/create/update/delete actions.
- `internal/api/starter` contains Starter cluster HTTP request/response models.
- `internal/db/validate` normalizes cluster names, cluster type, spending limit,
  and optional API-backed create parameters.
- `internal/dryrun` is used by create/update/delete to return validated request
  summaries without sending mutating HTTP requests.
- Delete safety is implemented by requiring an explicit cluster ID and reading
  the remote cluster before deletion, without prompting or requiring a display
  name confirmation flag.

## API Call Chain

Confirmed API base:

- `https://serverless.tidbapi.com`
- HTTP Digest auth with TiDB Cloud public/private API keys.

Command mapping:

- `tdc db list-db-clusters`
  1. Validate profile, provider, and region.
  2. Call `GET /v1beta1/clusters` with optional `filter`, `pageSize`,
     `pageToken`, `orderBy`, and `skip`.
  3. Filter or validate Starter-only behavior using returned cluster metadata.
- `tdc db create-db-cluster`
  1. Validate `--db-cluster-type starter`.
  2. Resolve explicit `--project-id` or the profile's configured `project_id`
     and set label `tidb.cloud/project`.
  3. Call `POST /v1beta1/clusters` with `displayName`, project label, region
     name such as `regions/aws-us-east-1`, and only other confirmed fields.
  4. When `--wait-until-active` is set and the create response is not already
     `ACTIVE`, call `GET /v1beta1/clusters/{clusterId}` every two seconds until
     `ACTIVE`, a terminal state, cancellation, polling failure, or the
     12-minute deadline.
- `tdc db describe-db-cluster`
  1. Call `GET /v1beta1/clusters/{clusterId}` with optional `view`.
  2. If the returned cluster exposes `clusterPlan` and it is not `STARTER`,
     return a validation error.
- `tdc db update-db-cluster`
  1. Call `GET /v1beta1/clusters/{clusterId}` and validate Starter-only
     behavior when `clusterPlan` is present.
  2. Call `PATCH /v1beta1/clusters/{clusterId}` with `updateMask` and the
     supported cluster fields being updated. MVP update fields are
     `displayName` and `spendingLimit`.
- `tdc db delete-db-cluster`
  1. Validate `--db-cluster-id`.
  2. Call `GET /v1beta1/clusters/{clusterId}`.
  3. If `clusterPlan` is present, verify it is `STARTER`.
  4. Call `DELETE /v1beta1/clusters/{clusterId}`.

Available but not part of this lifecycle MVP:

- `PUT /v1beta1/clusters/{clusterId}/password` for root password changes.
- `POST /v1beta1/clusters:batchCreate`.
- `GET /v1beta1/clusters:batchGet`.

## Dependencies And Platform

- No new third-party dependency beyond specs `0001` through `0004`.
- Uses the shared authenticated HTTP client and output/query layer.
- No cgo is required.
- Platform-neutral.

## Dependencies

- `0004-api-client-auth-and-region-routing.md`.
- `0003-output-error-query-dry-run.md`.

## Acceptance Criteria

- Mock API tests cover create/list/describe/update/delete.
- Mock API tests cover wait success, an immediately active create response,
  terminal state, polling failure, timeout, and default non-wait behavior.
- Tests cover required `--db-cluster-type starter` on create.
- Tests cover dry-run request validation and wait-plan reporting without
  sending mutating requests.
- Tests cover stable JSON output and `--query`.
- Tests cover delete safety behavior without prompts.
- `make live-e2e` covers the real cluster lifecycle: create a uniquely named
  `tdc-e2e-*` Starter cluster with `--wait-until-active` and without a spending
  limit, verify the create result is `ACTIVE`, read it, update it, read it
  again, delete it, and verify the cluster becomes deleted or not found. The
  cleanup path only deletes the cluster created by that test run.

## Out Of Scope

- Dedicated or Premium cluster-specific behavior.
- SQL user preparation and SQL execution, covered by
  `0008-starter-db-sql-access-and-query.md`.
- Import, export, backup, and audit-log commands unless added by a later spec.

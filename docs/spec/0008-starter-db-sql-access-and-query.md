# Starter DB SQL Access And Query

## Goal

Allow tdc to prepare database SQL users and execute one-shot SQL against Starter
clusters in an agent-friendly, deterministic way.

## User-facing Commands

Initial command set:

- `tdc db prepare-db-query-access`
- `tdc db create-db-connection-string`
- `tdc db execute-sql-statement`

Primary shapes:

```bash
tdc db prepare-db-query-access --db-cluster-id <cluster-id>
tdc db create-db-connection-string --db-cluster-id <cluster-id>
tdc db create-db-connection-string --db-cluster-id <cluster-id> --read-only --format env
tdc db create-db-connection-string --db-cluster-id <cluster-id> --admin --format jdbc
tdc db execute-sql-statement --db-cluster-id <cluster-id> --sql "select 1"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-write --sql "insert into t values (1)"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-only --sql "select * from t"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --admin --sql "show grants"
```

## Behavior

- `prepare-db-query-access` creates and stores three tdc-managed SQL users for the target
  cluster:
  - `read_only`, backed by TiDB Cloud built-in role `role_readonly`.
  - `read_write`, backed by TiDB Cloud built-in role `role_readwrite`.
  - `admin`, backed by TiDB Cloud built-in role `role_admin`.
- `prepare-db-query-access` must be idempotent and re-entrant. Re-running it must not
  create a new group of users when the expected tdc-managed users already exist.
- Use stable user suffixes per cluster, initially `tdc_ro`, `tdc_rw`, and
  `tdc_admin`. TiDB Cloud may add the cluster user prefix through the SQL user
  API; store the full returned username.
- If a tdc-managed user exists remotely but the local password is missing,
  `prepare-db-query-access` updates that user's password and writes the new
  password to local credentials instead of creating a duplicate user. This is
  backed by the confirmed SQL user PATCH API.
- SQL user GET/List responses do not expose passwords. Lost local passwords
  cannot be recovered from TiDB Cloud; `prepare-db-query-access` can only
  rotate/reset the password for a verified tdc-managed SQL user.
- A remote user is considered tdc-managed only when both the stable username
  suffix and the expected `builtinRole` match. If a matching suffix exists with
  a different role or auth method, fail with a conflict error instead of
  changing it.
- `create-db-connection-string` and `execute-sql-statement` use the
  `read_write` user by default.
- `create-db-connection-string --read-write` and
  `execute-sql-statement --read-write` explicitly use the `read_write` user.
- `create-db-connection-string --read-only` and
  `execute-sql-statement --read-only` use the `read_only` user.
- `create-db-connection-string --admin` and `execute-sql-statement --admin`
  use the `admin` user and must be explicitly specified.
- For both commands, `--read-only`, `--read-write`, and `--admin` are mutually
  exclusive.
- Do not infer access mode from SQL text. There is no `auto` mode.
- `create-db-connection-string` emits credential-bearing output. This is
  intentional when the user asks for a connection string, but the command must
  never send usernames, passwords, or full connection strings to telemetry.
- `create-db-connection-string` must support common connection string formats:
  - `mysql-uri`: `mysql://<user>:<password>@<host>:<port>/<database>?ssl-mode=VERIFY_IDENTITY`
  - `jdbc`: `jdbc:mysql://<host>:<port>/<database>?user=<user>&password=<password>&sslMode=VERIFY_IDENTITY`
  - `go-sql-driver`: `<user>:<password>@tcp(<host>:<port>)/<database>?tls=true&parseTime=true`
  - `sqlalchemy`: `mysql+pymysql://<user>:<password>@<host>:<port>/<database>?ssl_verify_identity=true`
  - `env`: dotenv-compatible key-value lines with connection components so
    agents can assemble the exact framework-specific value they need.
- `create-db-connection-string` defaults to `mysql-uri` unless `--format` is
  provided.
- `execute-sql-statement` executes exactly one SQL statement per invocation.
- HTTP SQL execution is the default transport.
- MySQL transport is supported as an explicit fallback path through a flag such
  as `--transport mysql`; it opens one connection, executes once, and closes the
  connection.
- Do not automatically retry write-capable queries on MySQL after an HTTP
  failure, because the HTTP request may already have been executed remotely.

## Inputs And Config

Common flags:

- `--db-cluster-id <id>` is required.
- `--database <name>` is optional and maps to the HTTP `TiDB-Database` header or
  MySQL default database. For connection strings, it maps to the path/default
  schema component; if omitted, use an empty path/default database only when
  the target format supports it.
- `--sql <statement>` is required for `tdc db execute-sql-statement`.
- `--read-only`, `--read-write`, and `--admin` select SQL user credentials.
- `--transport http|mysql` selects the execution transport. Default is `http`.
- `--format mysql-uri|jdbc|go-sql-driver|sqlalchemy|env` is valid for
  `tdc db create-db-connection-string`. Default is `mysql-uri`.
- `--env-prefix <prefix>` is valid only with `--format env`. Default is
  `TIDB_`.
- `--env-include-database-url` is valid only with `--format env`; when set,
  include a `DATABASE_URL` line in addition to component variables. The MVP
  default is false so agents can compose framework-specific connection strings
  from parts.
- `--env-database-url-name <name>` is valid only with
  `--env-include-database-url`; default is `DATABASE_URL`.
- `--dry-run` is valid for `prepare-db-query-access` only.

Store generated database SQL credentials outside the main profile credentials
file:

```text
~/.tdc/db_users/<cluster-id>/credentials
```

Do not store DB SQL users under `~/.tdc/credentials` as nested TOML tables.
`<cluster-id>` is globally unique in TiDB Cloud, so the path is intentionally
cluster-scoped instead of profile-scoped. The active profile only chooses which
TiDB Cloud API keys are used to prepare or repair the cluster's SQL users.
Keeping DB SQL credentials cluster-scoped avoids duplicate local passwords for
the same remote cluster when multiple profiles can access it.

Validate `<cluster-id>` as a single safe path segment before using it in a
filesystem path. Reject values containing path separators, `..`, empty strings,
or platform-specific invalid path characters.

```toml
# ~/.tdc/db_users/<cluster-id>/credentials
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

The DB user credentials file must use owner read/write permissions where the
platform supports POSIX mode bits. Parent directories under `~/.tdc/db_users/`
must not be world-readable where permission enforcement is available.

Do not store SQL text in config, credentials, telemetry, or logs.

## Output And Errors

- `prepare-db-query-access` returns JSON by default with user status per role:
  `created`, `exists`, `updated_password`, or `skipped_by_dry_run`.
- `create-db-connection-string` returns JSON by default with cluster ID, access
  mode, username, host, port, database, TLS mode, selected format, and the
  generated connection string. The password is included only inside the
  connection string field in JSON output, because the command exists to produce
  a usable secret-bearing connection value.
- `create-db-connection-string --format env` prints dotenv-compatible lines by
  default instead of JSON, because the output is intended to be redirected into
  an `.env` file or read by an agent. It must include component variables:
  `TIDB_HOST`, `TIDB_PORT`, `TIDB_USER`, `TIDB_PASSWORD`, `TIDB_DATABASE`,
  `TIDB_SSL_MODE`, `TIDB_ACCESS_MODE`, and `TIDB_CONNECTION_FORMAT`. It includes
  a URL variable only when `--env-include-database-url` is set. The env output
  prioritizes components so agents can compose framework-specific values.
- `execute-sql-statement` returns JSON by default with fields, rows, row count,
  rows affected, last insert ID, transport, access mode, and cluster ID.
- `execute-sql-statement --output human` may print a compact table for row
  results.
- Missing prepared credentials must suggest running
  `tdc db prepare-db-query-access`.
- Read-only user write failures should be reported as database permission
  errors, not as CLI validation errors.
- SQL text must not appear in telemetry. It may appear in command output only
  when the user explicitly requests full result metadata and the result contract
  documents it.

## After This Spec

Users can prepare a cluster once and then run SQL without passing usernames or
passwords on every command:

```bash
tdc db prepare-db-query-access --db-cluster-id <cluster-id>
tdc db create-db-connection-string --db-cluster-id <cluster-id> --read-write --format mysql-uri
tdc db create-db-connection-string --db-cluster-id <cluster-id> --read-only --format env
tdc db execute-sql-statement --db-cluster-id <cluster-id> --sql "select current_date()"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-write --sql "insert into audit_log values (1)"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --read-only --sql "select * from audit_log"
tdc db execute-sql-statement --db-cluster-id <cluster-id> --admin --sql "show grants"
```

Agents get deterministic privilege selection: default is read-write,
`--read-write` is available for explicitness, read-only and admin require
explicit flags, and tdc never guesses from SQL content.

## Implementation Design

- `internal/cli/db` registers `prepare-db-query-access`,
  `create-db-connection-string`, and `execute-sql-statement`.
- `internal/db/sqlaccess` owns idempotent SQL user preparation, stable username
  planning, password generation, local credential persistence, and repair of
  missing local passwords.
- `internal/db/sqlcred` defines the credentials schema under
  `~/.tdc/db_users/<cluster-id>/credentials` and loads credentials by cluster ID
  and access mode. It does not store DB SQL users in the main
  `~/.tdc/credentials` profile file.
- `internal/api/starter` adds SQL user list/get/create/update calls and cluster
  or branch endpoint retrieval needed for query execution.
- `internal/db/sqlhttp` implements the HTTP SQL transport based on the
  serverless driver shape: `POST https://http-<host>/v1beta/sql`, Basic Auth,
  `TiDB-Database`, and JSON body `{"query":"..."}`.
- `internal/db/sqlmysql` implements the explicit MySQL one-shot transport using
  `database/sql`.
- `internal/db/connectionstring` formats SQL credentials and cluster endpoint
  metadata into `mysql-uri`, `jdbc`, `go-sql-driver`, `sqlalchemy`, and `env`
  outputs. This package must percent-encode usernames, passwords, and database
  names where required by the target format.
- `internal/db/sqlresult` owns response decoding and stable result models.
- Passwords are generated with `crypto/rand`; do not use math/rand or timestamp
  suffixes for secrets.

## API Call Chain

Reference-derived TiDB Cloud SQL user APIs:

These endpoints come from `ref/tidbcloud-cli` generated IAM OpenAPI/client
files, not from confirmed public product docs. Use them for the MVP trial path,
but keep the implementation isolated so a live API mismatch returns a clear
API-gap error. Do not replace these calls with SQL `CREATE USER`/`ALTER USER`
fallbacks unless product explicitly accepts the security and privilege tradeoff.

- `GET /v1beta1/clusters/{clusterId}/sqlUsers`
- `POST /v1beta1/clusters/{clusterId}/sqlUsers`
- `GET /v1beta1/clusters/{clusterId}/sqlUsers/{userName}`
- `PATCH /v1beta1/clusters/{clusterId}/sqlUsers/{userName}`
- `DELETE /v1beta1/clusters/{clusterId}/sqlUsers/{userName}`

`prepare-db-query-access` call chain:

1. Load profile, TiDB Cloud API credentials, and existing local DB credentials.
2. Call `GET /v1beta1/clusters/{clusterId}` to verify the cluster exists and to
   read endpoint/user-prefix metadata needed by later query execution.
3. Call `GET /v1beta1/clusters/{clusterId}/sqlUsers` and match the stable
   tdc-managed suffixes `tdc_ro`, `tdc_rw`, and `tdc_admin`. Store and compare
   full returned usernames because TiDB Cloud may apply an automatic prefix.
   Confirm each match has the expected `builtinRole` and `authMethod`; role or
   auth-method mismatch is a conflict, not a repairable credential loss.
   Do not expect this API to return passwords.
4. For each missing remote user, generate a password with `crypto/rand` and call
   `POST /v1beta1/clusters/{clusterId}/sqlUsers` with:
   - `authMethod: "mysql_native_password"`
   - `autoPrefix: true`
   - `userName: "tdc_ro" | "tdc_rw" | "tdc_admin"`
   - `builtinRole: "role_readonly" | "role_readwrite" | "role_admin"`
   - `password: <generated>`
5. For each existing remote user whose local password is missing, generate a new
   password and call `PATCH
   /v1beta1/clusters/{clusterId}/sqlUsers/{userName}` with:
   - `password: <generated>`
   Do not create a duplicate user. If PATCH returns permission denied or a
   validation error, fail with an actionable error and leave the local
   credential entry unchanged.
6. Write the resulting full username and password for each access mode into
   `~/.tdc/db_users/<cluster-id>/credentials` atomically with owner-only file
   permissions.

HTTP query call chain:

1. Load the selected local DB credential. Default access mode is `read_write`;
   `--read-only`, `--read-write`, and `--admin` are mutually exclusive.
2. Call `GET /v1beta1/clusters/{clusterId}` if the HTTP host is not already
   cached in local non-secret config. Use the public endpoint host from the
   returned cluster resource.
3. Send `POST https://http-<host>/v1beta/sql`.
4. Authenticate with HTTP Basic Auth using the generated SQL username and
   password.
5. Send headers:
   - `Content-Type: application/json`
   - `User-Agent: tdc/<version>`
   - `TiDB-Database: <database-or-empty>`
   - `TiDB-Session: ""` for one-shot stateless execution
   - `X-Debug-Trace-Id: <generated-request-id>` when debug tracing is enabled
6. Send body `{"query":"<sql>"}`.
7. Decode response fields equivalent to the serverless HTTP SQL result:
   `types`, `rows`, `rowsAffected`, `sLastInsertID`, and response
   `TiDB-Session`. tdc does not persist the session for one-shot queries.

Connection string call chain:

1. Load the selected local DB credential. Default access mode is `read_write`;
   `--read-only`, `--read-write`, and `--admin` are mutually exclusive.
2. Call `GET /v1beta1/clusters/{clusterId}` if the public MySQL host/port is
   not already cached in local non-secret config. Use the public endpoint host
   and port from the returned cluster resource.
3. Validate `--format` and `--database` for the selected output format.
4. Format the connection output:
   - `mysql-uri` and `sqlalchemy` use URI percent-encoding for username,
     password, and database.
   - `jdbc` percent-encodes query parameter values and keeps the host/port in
     authority form.
   - `go-sql-driver` uses the DSN form expected by `github.com/go-sql-driver/mysql`.
   - `env` outputs dotenv-compatible component variables and quotes values that
     contain spaces, `#`, quotes, or shell-sensitive characters.
5. Return the requested output without writing the connection string to config,
   credentials, logs, or telemetry.

MySQL fallback call chain:

1. Use the same selected SQL credential and cluster public MySQL endpoint.
2. Open a `database/sql` connection through `github.com/go-sql-driver/mysql`.
3. Execute exactly one statement.
4. Close the connection before returning.

## Dependencies And Platform

- Add `github.com/go-sql-driver/mysql` for explicit MySQL fallback transport.
- HTTP transport uses Go standard library `net/http`.
- Result decoding uses Go standard library `encoding/json`.
- No cgo is required.
- MySQL fallback is cross-platform but should stay optional at runtime.
- Do not depend on `ref/serverless-js`; copy protocol behavior only.

## Dependencies

- `0006-starter-db-cluster-lifecycle.md`.
- `0004-api-client-auth-and-region-routing.md`.
- `0003-output-error-query-dry-run.md`.

## Acceptance Criteria

- `prepare-db-query-access` creates read-only, read-write, and admin users when none
  exist.
- Re-running `prepare-db-query-access` with existing users does not create duplicate
  users.
- If local credentials are missing but remote tdc-managed users exist,
  `prepare-db-query-access` updates passwords and stores them locally.
- Prepared DB SQL credentials are stored in
  `~/.tdc/db_users/<cluster-id>/credentials`, not in the main
  `~/.tdc/credentials` file.
- Tests reject unsafe cluster IDs before constructing DB user credential paths.
- Tests cover DB user credential file creation, atomic update behavior, and
  owner-only permissions where the platform supports them.
- Query defaults to read-write credentials.
- Query supports explicit `--read-write` for deterministic agent workflows.
- Query uses read-only credentials only when `--read-only` is specified.
- Query uses admin credentials only when `--admin` is specified.
- Connection string generation defaults to read-write credentials.
- Connection string generation supports explicit `--read-write`, `--read-only`,
  and `--admin` access-mode flags.
- Connection string generation supports `mysql-uri`, `jdbc`, `go-sql-driver`,
  `sqlalchemy`, and `env` formats.
- Env output includes component variables so agents can assemble
  framework-specific connection strings without parsing a URL.
- Tests reject combined access-mode flags.
- Tests verify no SQL text, usernames, passwords, or connection strings are sent
  to telemetry.
- HTTP transport tests verify URL, Basic Auth, headers, body, success response,
  and error response handling.
- MySQL transport tests verify one connection is opened and closed per query.

## Out Of Scope

- Interactive SQL shell.
- Branch SQL query execution until branch SQL user semantics are verified.
- Stateful HTTP sessions and transactions.
- SQL parsing or automatic read/write classification.
- User-managed SQL users outside the tdc-managed credential set.

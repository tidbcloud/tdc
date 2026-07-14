# Serverless Function Deployment

## Goal

Add an agent-friendly serverless function workflow that can package a local HTTP
function and deploy it to supported external runtimes such as Vercel Functions
and AWS Lambda.

tdc should not try to become a serverless platform in this spec. It should be a
predictable packaging and deployment adapter: local source plus an explicit
manifest becomes a provider-specific deployment, and the CLI reports the
resulting function URL and metadata in structured output.

## User-facing Commands

Add a new top-level command namespace:

```bash
tdc function init-function
tdc function validate-function
tdc function package-function
tdc function deploy-function
tdc function describe-function
tdc function list-functions
tdc function delete-function
tdc function invoke-function
tdc function get-function-url
tdc function logs-function
```

Initial MVP commands may implement only `init-function`, `validate-function`,
`package-function`, `deploy-function`, `invoke-function`, and
`get-function-url`; the remaining commands can be registered as placeholders
until provider state management is implemented.

Example:

```bash
tdc function init-function --function-name hello --runtime nodejs --target vercel-node
tdc function validate-function --function-file tdc.function.toml
tdc function package-function --function-file tdc.function.toml --output-dir .tdc/functions/hello
tdc function deploy-function --function-file tdc.function.toml --target vercel-node
tdc function get-function-url --function-name hello --target vercel-node
tdc function invoke-function --function-url https://example.vercel.app/api/hello
```

## Behavior

- Functions are HTTP request/response functions in the MVP.
- The user must choose an explicit deployment target. Do not auto-detect whether
  a function should deploy to Vercel Node.js, Vercel Edge, AWS Lambda ZIP, or
  AWS Lambda container.
- The default local function signature should be Web Fetch style where the
  runtime supports it:

```ts
export default {
  async fetch(request: Request) {
    return new Response("hello");
  },
};
```

- `validate-function` performs local checks only: manifest schema, file
  existence, runtime/target compatibility, required provider credentials, and
  forbidden secret placement.
- `package-function` creates provider-specific build output without deploying.
- `deploy-function` validates, packages, deploys, records local deployment
  metadata, and outputs structured JSON by default.
- `invoke-function` performs a single HTTP request against the function URL.
- `get-function-url` returns the recorded function URL when available.
- Mutating commands support `--dry-run`: `deploy-function` and
  `delete-function` must validate local inputs and planned provider operations
  without calling provider mutation APIs.
- Read-only commands reject `--dry-run`.
- Successful structured commands support `--output json|text` and `--query`.
- Do not prompt except inside existing `tdc configure`. Missing provider
  credentials must fail with actionable errors.

## Inputs And Config

Each function is described by a local manifest:

```toml
# tdc.function.toml
name = "hello"
runtime = "nodejs"
entrypoint = "src/hello.ts"
handler = "fetch"
target = "vercel-node"

[http]
path = "/api/hello"
methods = ["GET", "POST"]

[env]
DATABASE_URL = "secret:database_url"
```

Supported MVP manifest fields:

- `name`: stable function name.
- `runtime`: `nodejs`, `python`, or `go` depending on target support.
- `entrypoint`: local source entrypoint.
- `handler`: target-specific handler name or `fetch`.
- `target`: explicit deployment target.
- `[http].path`: HTTP path when the target supports routing.
- `[http].methods`: allowed methods for generated adapter code when supported.
- `[env]`: environment variable bindings. Values starting with `secret:` refer
  to provider-side secrets or tdc-managed secret references and must not be
  inlined into package artifacts.

Provider credentials are not stored in the function manifest. They come from
provider-specific environment variables or future `~/.tdc/credentials` profile
keys.

Initial provider credential inputs:

- Vercel:
  - `VERCEL_TOKEN`
  - `VERCEL_ORG_ID` if required
  - `VERCEL_PROJECT_ID` if deploying into an existing project
- AWS:
  - standard AWS credential chain as used by the AWS SDK
  - `AWS_REGION` or explicit command flag where needed
  - ECR repository input for container deployments

Local deployment metadata lives under `~/.tdc/functions/`:

```text
~/.tdc/functions/<profile>/<function-name>/<target>/deployment.json
```

This metadata may store provider deployment IDs, function URLs, target, region,
runtime, source digest, and deploy time. It must not store provider tokens,
secret values, request bodies, response bodies, local absolute source paths, or
function source code.

## Target Matrix

MVP should start with two targets:

| Target | Runtime shape | Notes |
| --- | --- | --- |
| `vercel-node` | Vercel Node.js Function | Best first target for TypeScript/JavaScript HTTP handlers. |
| `aws-lambda-container` | AWS Lambda container image | Most flexible AWS path for Go, Python, Node.js, and custom dependencies. Requires Docker and ECR. |

Future targets:

| Target | Runtime shape | Notes |
| --- | --- | --- |
| `vercel-edge` | V8 isolate Edge Function | Low-latency and lightweight, but limited APIs; not all Node.js modules work. |
| `aws-lambda-zip` | AWS managed runtime ZIP | Simpler than containers for Node/Python, but less uniform across languages. |

Do not claim one source file is fully portable across all targets. The manifest
and validation must make runtime/target constraints explicit.

## Output And Errors

Example deploy JSON:

```json
{
  "function_name": "hello",
  "target": "vercel-node",
  "runtime": "nodejs",
  "status": "deployed",
  "url": "https://example.vercel.app/api/hello",
  "deployment_id": "dpl_...",
  "source_digest": "sha256:...",
  "metadata_stored": true
}
```

Example text output:

```text
Function: hello
Target: vercel-node
Runtime: nodejs
Status: deployed
URL: https://example.vercel.app/api/hello
```

Example validation error:

```text
tdc [ERROR]: target vercel-edge does not support runtime nodejs with Node built-in module "fs"; use vercel-node or remove the dependency
```

Example missing credentials error:

```text
tdc [ERROR]: authentication required: VERCEL_TOKEN is required to deploy target vercel-node
```

## After This Spec

Users can create and deploy a function from a local project:

```bash
tdc function init-function --function-name hello --runtime nodejs --target vercel-node
tdc function validate-function
tdc function deploy-function
tdc function get-function-url --function-name hello
tdc function invoke-function --function-name hello --method GET
```

CI can package and deploy non-interactively:

```bash
tdc function validate-function --function-file tdc.function.toml
tdc function package-function --function-file tdc.function.toml --output-dir .tdc/build/function
tdc function deploy-function --function-file tdc.function.toml --target aws-lambda-container --dry-run
tdc function deploy-function --function-file tdc.function.toml --target aws-lambda-container
```

## Implementation Design

- `internal/function/manifest` owns TOML parsing, validation, defaults, schema
  versioning, and target compatibility checks.
- `internal/function/package` owns build output creation and source digesting.
- `internal/function/provider` defines a provider interface:

```go
type Provider interface {
    Validate(context.Context, DeployRequest) error
    Package(context.Context, PackageRequest) (PackageResult, error)
    Deploy(context.Context, DeployRequest) (DeployResult, error)
    Delete(context.Context, DeleteRequest) (DeleteResult, error)
    Logs(context.Context, LogsRequest) (LogsResult, error)
}
```

- `internal/function/provider/vercel` implements Vercel deployment.
- `internal/function/provider/aws` implements AWS Lambda deployment.
- `internal/function/state` stores local deployment metadata under
  `~/.tdc/functions/`.
- `internal/cli` registers `tdc function ...` commands and keeps handlers thin.
- The first implementation should prefer provider CLIs or official SDKs only
  where their auth and deployment flows are stable. Avoid scraping web output.

Packaging strategy:

- `vercel-node` may generate a Vercel-compatible project or Build Output API
  directory, then deploy through the Vercel API or CLI.
- `aws-lambda-container` builds an OCI image, pushes it to ECR, creates or
  updates a Lambda function, and optionally creates a Lambda Function URL.
- All packaging must happen in a deterministic output directory under `.tdc/`
  or a user-specified `--output-dir`.
- Generated adapters should be small and explicit. Do not rewrite user source
  files in place.

## API Call Chain

Vercel deploy flow, high level:

1. Read `tdc.function.toml`.
2. Validate target/runtime compatibility.
3. Build provider output for Vercel.
4. Authenticate with `VERCEL_TOKEN`.
5. Create or update the Vercel deployment.
6. Record deployment URL and metadata.

AWS Lambda container deploy flow, high level:

1. Read `tdc.function.toml`.
2. Validate Docker, AWS credentials, AWS region, and ECR repository settings.
3. Build OCI image.
4. Push image to ECR.
5. Create or update Lambda function with package type image.
6. Create or update Function URL when requested.
7. Record function ARN, URL, image digest, and metadata.

## Dependencies And Platform

Potential Go dependencies:

- Vercel: prefer direct HTTPS client first if the API surface is small and
  stable; otherwise shell out to `vercel` CLI only when installed and explicitly
  requested.
- AWS: use AWS SDK for Go v2 for Lambda and ECR APIs.

External tools:

- `aws-lambda-container` requires Docker or a compatible OCI builder.
- `aws-lambda-container` requires network access to ECR.
- `vercel-node` may require Node.js/npm/pnpm depending on project build.
- `vercel-edge` future support requires stricter runtime validation because
  Edge runtime APIs differ from full Node.js.

Platform notes:

- Packaging should work on macOS and Linux first.
- Windows support is desired but may be limited by Docker and shell behavior in
  the first implementation.
- Do not introduce cgo requirements into the tdc binary.

## Dependencies

- `0001-cli-foundation.md` for command patterns.
- `0002-local-config-and-credentials.md` for profile and local state rules.
- `0003-output-error-query-dry-run.md` for output, query, and dry-run behavior.
- `0014-tdc-fs-unix-command-aliases.md` is unrelated but should remain
  unaffected.

## Acceptance Criteria

- `tdc function init-function` creates a minimal `tdc.function.toml` and sample
  source file without overwriting existing files unless `--overwrite` is set.
- `tdc function validate-function` rejects invalid target/runtime combinations
  before any provider API call.
- `tdc function package-function` writes deterministic provider output and
  reports a source digest.
- `tdc function deploy-function --dry-run` validates inputs and planned provider
  operations without mutating remote state.
- `tdc function deploy-function --target vercel-node` deploys a simple HTTP
  function and returns a URL.
- `tdc function deploy-function --target aws-lambda-container` deploys a simple
  HTTP function to Lambda container and returns a Function URL when configured.
- `tdc function invoke-function` can call the returned URL and print status,
  headers, and body according to output mode.
- Local deployment metadata is stored under `~/.tdc/functions/` without secrets.
- `make test` covers manifest validation, packaging decisions, state storage,
  and dry-run behavior without live provider credentials.
- Live provider tests, if added, are opt-in and isolated behind explicit
  environment variables. They must create uniquely named functions and clean up
  only resources created by the test run.
- README and AGENTS are updated when commands are implemented.

## Out Of Scope

- Running a self-hosted serverless platform.
- Automatic migration of arbitrary existing applications.
- Background jobs, queues, schedules, cron triggers, WebSockets, or streaming
  functions.
- Durable function state, retries, workflows, or orchestration.
- Provider billing management.
- Provider secret creation beyond references needed for deployment.
- Full cross-target source compatibility.
- Cloudflare Workers, Netlify Functions, Fly Machines, Google Cloud Run, Azure
  Functions, or other targets.
- Telemetry for function deployments.

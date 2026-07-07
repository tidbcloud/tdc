# GitHub Actions CI/CD

## Goal

Run repeatable automated checks on GitHub Actions after the MVP has been
manually tested. CI must protect ordinary pull requests from regressions without
requiring live TiDB Cloud credentials, and must provide a separate opt-in live
e2e path for the full cloud-backed suite.

This spec is intentionally after the install/update distribution work: before
enabling cloud-backed CI, the MVP workflows should be manually validated first.

## User-facing Commands

CI/CD executes the same local commands users and agents run:

- `make build`
- `make test`
- `make e2e`
- `make live-e2e`
- `bin/tdc configure --profile live-e2e --non-interactive`

No new product command is introduced by this spec.

## Behavior

- Add GitHub Actions workflows under `.github/workflows/`.
- Ordinary CI runs on pull requests and pushes to `main`.
- Ordinary CI must not require TiDB Cloud credentials or any live service.
- Ordinary CI runs:
  - dependency download
  - formatting check
  - `go mod tidy` cleanliness check
  - `make test`
  - `make e2e`
  - `make build`
- Live e2e runs through one opt-in workflow using `make live-e2e`.
- Do not add separate mutating/non-mutating live workflows. `make live-e2e` is
  the full live suite.
- Live e2e uses the special `live-e2e` profile.
- Live e2e should run only on `workflow_dispatch` at first. A scheduled run can
  be added later after the suite proves stable and cleanup-safe.
- Live e2e reads repository-level GitHub Actions secrets and variables. It does
  not require a GitHub Environment in the MVP workflow.
- Live e2e must generate unique test resource names and must attempt cleanup
  even when test assertions fail.
- Live e2e creates a temporary tdc fs resource before running `make live-e2e`
  and deletes that resource in an `always()` cleanup step.
- CI logs must not print public/private key values, generated DB passwords,
  connection strings with passwords, SQL results containing user data, or tdc fs
  file contents.

## Inputs And Config

Ordinary CI requires no secrets.

Live e2e reads repository-level GitHub Actions secrets and variables:

- `TDC_PUBLIC_KEY` secret
- `TDC_PRIVATE_KEY` secret
- `TDC_CLOUD_PROVIDER` variable, for example `aws`
- `TDC_REGION_CODE` variable, for example `us-east-1`

The workflow configures the live profile non-interactively:

```bash
TDC_CLOUD_PROVIDER="$TDC_CLOUD_PROVIDER" \
TDC_REGION_CODE="$TDC_REGION_CODE" \
TDC_PUBLIC_KEY="$TDC_PUBLIC_KEY" \
TDC_PRIVATE_KEY="$TDC_PRIVATE_KEY" \
bin/tdc configure --profile live-e2e --non-interactive
```

Then it runs:

```bash
FS_NAME="tdc-live-e2e-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
bin/tdc fs create-file-system --profile live-e2e --file-system-name "$FS_NAME"
make live-e2e
bin/tdc fs delete-file-system --profile live-e2e --file-system-name "$FS_NAME" --confirm-file-system-name "$FS_NAME"
```

The workflow must not commit or upload `~/.tdc/` as an artifact.

## Output And Errors

- Failed ordinary CI should show the failing command and test package.
- Failed live e2e should show the high-level failing workflow step and tdc error
  category, but must not reveal secret values.
- Test artifacts may include redacted logs and JUnit-style test reports if
  useful, but raw credentials, connection strings with passwords, and file
  payloads must be excluded.
- GitHub Actions log masking must be used for all configured secrets before any
  command that could echo environment variables.

## After This Spec

Contributors can rely on GitHub Actions for local-quality checks:

```bash
make test
make e2e
```

Maintainers can manually start the full live suite from GitHub Actions after
setting the repository-level variables and secrets:

```bash
make live-e2e
```

The live suite validates the real integration path using the same `live-e2e`
profile convention used locally and in CI/CD docs.

## Implementation Design

- Add `.github/workflows/ci.yml` for ordinary checks.
- Add `.github/workflows/live-e2e.yml` for opt-in live checks.
- Use `actions/checkout` and `actions/setup-go` with `go-version-file: go.mod`.
- Cache Go modules and build cache through the supported setup-go cache
  behavior.
- Prefer pinned action major versions or pinned SHAs, based on project release
  policy at implementation time.
- Add a formatting check that fails when `gofmt` would change Go files.
- Add a tidy check that fails when `go mod tidy` changes `go.mod` or `go.sum`.
- Keep all `ref/` directories excluded from build, test, release, and artifact
  packaging flows.
- Ordinary CI should run on `ubuntu-latest` first. Cross-platform build jobs for
  Linux, macOS, and Windows can be added after the CLI distribution workflow in
  `0012-install-and-update-distribution.md` is implemented.
- Live e2e should run on `ubuntu-latest` first unless a later command requires a
  platform-specific runner.
- Live tests should use the existing `make live-e2e` target and should not
  duplicate live test orchestration in workflow YAML.
- The live workflow installs Linux FUSE prerequisites before running the suite.
  If GitHub-hosted runners cannot provide usable `/dev/fuse`, mount coverage
  should move to a FUSE-capable runner instead of weakening `make live-e2e`.

## API Call Chain

Ordinary CI uses no TiDB Cloud API.

Live e2e exercises the API chains implemented by previous specs through the
compiled `tdc` binary:

- configure/profile loading from `0002-local-config-and-credentials.md`
- auth and region routing from `0004-api-client-auth-and-region-routing.md`
- organization lookup from `0005-organization-management.md`
- DB cluster and branch workflows from `0006` and `0007`
- DB SQL user preparation and SQL query workflow from `0008`
- tdc fs control plane, data plane, and mount runtime from `0009` through `0011`
- install/update smoke coverage from `0012`

## Dependencies And Platform

- GitHub Actions hosted runners.
- Go version comes from `go.mod`.
- No new runtime dependency is required.
- Workflow-only dependencies are limited to GitHub Actions actions and shell
  commands available on hosted runners.
- Ordinary CI must remain cgo-free unless a prior spec introduces a
  platform-specific mount job behind build tags.

## Dependencies

- `0001-cli-foundation.md` through `0012-install-and-update-distribution.md`.
- Manual MVP validation should happen before enabling the live e2e workflow as a
  required gate.

## Acceptance Criteria

- `.github/workflows/ci.yml` runs on pull requests and pushes to `main`.
- Ordinary CI passes without TiDB Cloud credentials.
- Ordinary CI checks formatting, module tidiness, unit tests, e2e tests, and
  build.
- `.github/workflows/live-e2e.yml` runs only through `workflow_dispatch`.
- Live e2e uses repository-level secrets and variables plus the `live-e2e` tdc
  profile.
- Live e2e configures tdc through `bin/tdc configure --profile live-e2e
  --non-interactive`.
- Live e2e creates one temporary `tdc-live-e2e-*` tdc fs resource before the
  suite and attempts to delete it after the suite with `if: always()`.
- Live e2e invokes `make live-e2e`.
- Workflow logs do not expose TiDB Cloud keys, generated DB credentials, or
  connection strings with passwords.
- Live e2e cleanup is tested or explicitly verified for created cloud
  resources.
- README documents how maintainers configure repository-level variables/secrets
  and run live e2e once the workflows are implemented.

## Out Of Scope

- Making live e2e a required pull request check before manual MVP validation.
- Additional release publishing, Homebrew publishing, Scoop publishing, package
  signing, or installer upload beyond the GoReleaser/GitHub Releases workflow
  from `0012`. Homebrew and Scoop belong to `0016`.
- Replacing local `make test`, `make e2e`, or `make live-e2e` with
  GitHub-only logic.

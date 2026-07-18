# PingCAP tdc Documentation

## Goal

Publish complete Preview documentation for tdc in the English and Chinese PingCAP documentation repositories. The documentation must help a new user reach a successful result quickly, cover every implemented command, explain the tdc and Drive9 ownership boundary, and provide scenario-based examples for database, filesystem, Git workspace, journal, vault, and ephemeral agent workflows.

The English documentation lives in the `pingcap/docs` submodule at `docs/pingcap-docs/docs/`. The Chinese translation lives in the `pingcap/docs-cn` submodule at `docs/pingcap-docs/docs-cn/`. Both submodules track the `release-8.5` branch and must keep matching document structures and links.

This is a documentation-only requirement. It does not add or change tdc runtime behavior, TiDB Cloud APIs, Drive9 APIs, configuration formats, credentials, commands, or dependencies.

## Product Status And Feedback

tdc is documented uniformly as **Preview**, not General Availability or Public Preview.

Every tdc documentation page must include the following note near the beginning of the page.

English:

```markdown
> **Note:**
>
> tdc is currently in Preview. Its features and command-line interface might change without prior notice.
```

Chinese:

```markdown
> **注意：**
>
> tdc 当前处于预览（Preview）阶段，其功能和命令行界面可能会发生变更，恕不另行通知。
```

The tdc section labels in both `TOC-ai.md` files and both AI home pages must also include `(Preview)`.

Product feedback and bug reports must point to:

```text
https://github.com/tidbcloud/tdc/issues
```

Do not direct ordinary tdc feedback to the TiDB, Drive9, or PingCAP docs issue trackers.

## File Naming And Layout

All new tdc Markdown basenames must be globally unique within each PingCAP documentation repository, even when files are stored in different directories. Every basename must start with the `tdc-` prefix and use the existing PingCAP docs kebab-case naming convention. Do not add a generic `_index.md`, `overview.md`, `quick-start.md`, or `reference.md` under the tdc tree.

The English and Chinese repositories use the same relative paths:

```text
ai/tdc/
  tdc-overview.md
  tdc-quick-start.md
  concepts/
    tdc-concepts-and-architecture.md
  guides/
    tdc-install-configure-update.md
    tdc-organization.md
    tdc-starter-database.md
    tdc-filesystem.md
    tdc-filesystem-git.md
    tdc-filesystem-journal.md
    tdc-filesystem-vault.md
  examples/
    tdc-agent-sandbox-example.md
    tdc-daily-workflow-example.md
    tdc-query-sql-with-roles-example.md
    tdc-share-filesystem-across-machines-example.md
    tdc-git-workspace-for-agents-example.md
    tdc-journal-agent-workflow-example.md
    tdc-vault-agent-secrets-example.md
  reference/
    tdc-cli-reference.md
    tdc-configuration-and-credentials.md
    tdc-regions-security-and-limitations.md
    tdc-troubleshooting.md
```

All internal links use absolute PingCAP documentation paths rooted at `/ai/tdc/`. English and Chinese links use the same paths; locale routing is handled by the documentation site.

## Documentation Style

Follow the existing PingCAP documentation templates and AI documentation style:

- Every page has YAML front matter with an SEO-oriented `title` and `summary`.
- The front matter title and the page H1 are identical.
- Task pages use a short introduction, prerequisites, numbered steps, expected results, cleanup where resources are created, and a "What's next" section.
- Concept and reference pages use the corresponding PingCAP concept/reference templates.
- Notes and warnings use the standard blockquote format used by the PingCAP docs repositories.
- Prose is concise and task-oriented. Do not add manual line breaks inside ordinary paragraphs without a semantic reason.
- Commands use the installed `tdc` name, never the local development path `bin/tdc`.
- Examples use placeholders and synthetic resource names. Never include real API keys, FS tokens, SQL passwords, vault tokens, project IDs, cluster IDs, tenant IDs, or customer data.
- Secrets should be passed through environment variables where possible. If a secret flag must be explained, warn that flags may remain in shell history or process listings.
- English is the source documentation. Chinese pages preserve the same technical meaning and command blocks but use natural Chinese technical writing rather than literal machine translation.
- Product and command names remain in their canonical form: `tdc`, TiDB Cloud Starter, TiDB Cloud Filesystem, `tdc fs`, `tdc fs-git`, `tdc fs-journal`, and `tdc fs-vault`.

Do not document `make build`, `go build`, `go install`, or source compilation as an end-user installation method. Developers who need source builds can use the tdc GitHub repository. Published user installation documentation covers only supported release installers and updates.

## Source Of Truth

Documentation must describe the current implementation, not historical demos or superseded specs.

Use these sources in priority order:

1. The current compiled CLI help for command names, required and optional flags, aliases, and descriptions.
2. Current code and tests for behavior, precedence, error handling, output, and platform differences.
3. `README.md` and `AGENTS.md` after they have been audited against the current binary.
4. Current completed specs that have not been superseded.
5. Historical completed specs and `docs/present.md` only as scenario context after validating every command.

Do not publish commands that are absent from the current CLI. In particular, do not document `tdc fs-git restore-git-workspace`; Drive9 does not expose that operation through the wrapped public CLI surface.

The implementation must regenerate a command inventory before writing the command guides. At the time this spec is written, the current surface contains:

- two top-level operational commands: `tdc configure` and `tdc update`;
- 64 subcommands across `organization`, `db`, `fs`, `fs-git`, `fs-journal`, and `fs-vault`;
- 15 Unix-style aliases under `tdc fs`;
- global help, version, profile, region, output, query, and debug behavior.

The final documentation must follow the actual surface at implementation time if this count changes.

## Quick Start

`tdc-quick-start.md` optimizes for the shortest path to a successful result. It must not attempt to summarize all tdc features.

The quick start covers:

1. Install tdc with the supported shell or PowerShell release installer.
2. Add the user-owned tdc binary directory to the current shell `PATH`.
3. Run interactive `tdc configure` using a TiDB Cloud API public/private key pair and canonical region code.
4. Verify configuration with one non-mutating command.
5. Let the user choose one short Starter database path or one short Filesystem path.

The Filesystem quick path uses data-plane commands to write and read one file. It must not require FUSE, macFUSE, a mount, layers, Git, journal, or vault.

The database quick path creates or selects a Starter cluster, prepares SQL users when needed, and executes a small verification query with an explicit role. Keep resource-ID extraction and cleanup understandable; do not require users to understand every output/query option before their first successful operation.

## Concepts

`tdc-concepts-and-architecture.md` explains only concepts needed to understand later guides:

- tdc as the TiDB Cloud CLI for Starter databases and TiDB Cloud Filesystem;
- the two-level command model and agent-friendly deterministic behavior;
- profile namespace, canonical region code, and default virtual project;
- one profile to many Filesystem resources;
- TiDB Cloud API keys versus generated DB SQL credentials;
- read-only, read-write, and admin SQL roles;
- FS owner token versus delegated vault token;
- local config, credentials, resource registry, DB user credentials, mount state, and operation logs;
- tdc and Drive9 companion responsibilities.

tdc bundles and invokes the Drive9 companion as `tdc-drive9`. tdc owns profile selection, credential resolution, region routing, resource selection, output, and error behavior. Drive9 owns Filesystem data-plane semantics, FUSE/WebDAV mount runtime, layers, pack/unpack, Git workspace, journal, and vault behavior.

Link the Drive9 name to its GitHub repository:

```text
https://github.com/mem9-ai/drive9
```

Users must not be instructed to install, configure, authenticate, or invoke standalone Drive9 for normal tdc workflows.

## Guides

The guides cover every implemented command, grouped by the existing top-level command families.

### Install, configure, and update tdc

`tdc-install-configure-update.md` covers:

- release installer usage on macOS, Linux, and Windows;
- installation under the user-owned tdc binary directory;
- `PATH` setup without sudo or system-directory symlinks;
- interactive and non-interactive `tdc configure`;
- profile and command-scope region selection;
- `tdc update --check`, `tdc update --dry-run`, `tdc update`, and target-version updates;
- help and version behavior;
- safe uninstall instructions that distinguish binaries from optional removal of `~/.tdc/` user state.

Do not include source-build instructions.

### Organization

`tdc-organization.md` covers all `tdc organization` commands, project output, `tidbx` and `tidbx_virtual` project types, and configure-time default virtual-project discovery.

### Starter database

`tdc-starter-database.md` covers all `tdc db` commands:

- Starter cluster create, list, describe, update, and delete;
- optional/default project selection;
- cluster branch create, list, describe, and delete;
- idempotent creation or repair of tdc-managed SQL users;
- read-only, read-write, and admin role selection;
- connection string formats;
- HTTPS SQL execution and explicit MySQL fallback;
- one-statement execution behavior, output modes, dry-run support, and cleanup.

### Filesystem

`tdc-filesystem.md` covers all `tdc fs` commands and Unix aliases:

- create, list, describe, check, select default, unset default, and delete Filesystem resources;
- one profile to many resources and deterministic resource-selection precedence;
- FS token output and handling as a secret;
- config-free token access;
- copy, read, list, describe, move, delete, mkdir, chmod, symlink, hardlink, search, and find operations;
- range read, append, resume, recursive copy, stdin/stdout, tags, and descriptions where exposed by current help;
- layers and checkpoints;
- pack and unpack;
- mount, drain, and unmount;
- FUSE and WebDAV behavior;
- the canonical commands and their 15 Unix-style aliases.

The macOS behavior must be explicit:

- WebDAV is the default macOS mount path.
- The default WebDAV path works without macFUSE.
- Users can install macFUSE and explicitly select FUSE to get the complete Filesystem mount experience.
- Link to the official macFUSE site at `https://macfuse.github.io/` and explain any current approval/restart prerequisites without copying unsupported instructions.
- Show the exact tdc command for explicitly selecting FUSE after installation.
- Do not describe a tdc-owned mount implementation; mount behavior is delegated to the bundled Drive9 companion.

The guide must also provide a concise platform matrix for macOS, Linux, and Windows based on current tested behavior.

### Filesystem Git, journal, and vault

`tdc-filesystem-git.md` covers clone, hydrate, add-worktree, and remove-worktree. It explains that tdc FS Git augments ordinary Git workspaces and does not replace Git.

`tdc-filesystem-journal.md` covers journal create, append, read, search, and hash-chain verification. It explains why a journal is an append-only verifiable workflow ledger rather than an ordinary mutable text file.

`tdc-filesystem-vault.md` covers secret create, replace, read, list, and delete; delegated grants; grant revocation; audit events; environment injection; read-only vault mount; and unmount. It distinguishes owner FS credentials from delegated vault tokens and must not print a token in an example output.

## Examples

Examples are end-to-end scenarios rather than command catalogs. Every example includes prerequisites, commands, expected verification, security notes, and cleanup.

- `tdc-agent-sandbox-example.md`: provision on a trusted machine, pass `TDC_FS_TOKEN`, `TDC_REGION_CODE`, and `TDC_FS_FILE_SYSTEM_NAME` to a clean sandbox, then use FS, mount, Git, journal, or vault without TiDB Cloud API keys.
- `tdc-daily-workflow-example.md`: install, configure, inspect projects, manage one Starter cluster and one Filesystem resource, update tdc, and clean up.
- `tdc-query-sql-with-roles-example.md`: prepare users, execute SQL with explicit read-only/read-write/admin roles, and format a connection string without exposing credentials.
- `tdc-share-filesystem-across-machines-example.md`: create a Filesystem on one machine, transfer the owner token through a secure channel, access it from another machine, verify mount/data-plane visibility, then drain and unmount.
- `tdc-git-workspace-for-agents-example.md`: mount a Filesystem, clone or hydrate a repository, create a worktree, use ordinary Git, and remove the worktree safely.
- `tdc-journal-agent-workflow-example.md`: create a journal, append agent events, search them, and verify the hash chain.
- `tdc-vault-agent-secrets-example.md`: create a secret, delegate a limited field to an agent, inject it into a process, inspect audit events, revoke access, and clean up.

Examples that mount on macOS default to WebDAV unless the example specifically demonstrates the optional macFUSE/FUSE path.

## Reference

`tdc-cli-reference.md` documents global flags, long-flag rules, required-before-optional help ordering, JSON/text output, JMESPath queries, dry-run behavior, help/version forms, stable error prefix, exit behavior, and FS alias mapping.

`tdc-configuration-and-credentials.md` documents:

- `~/.tdc/` state ownership;
- profile, credential, placement, Filesystem selection, and token precedence;
- config and credential paths;
- per-resource Filesystem registry paths;
- cluster-scoped DB SQL credential paths;
- config-free in-memory inputs;
- mount locators;
- local operation logs and `TDC_LOGGING=off`;
- which values are sensitive and must never be logged.

`tdc-regions-security-and-limitations.md` documents supported canonical TiDB Cloud region codes, current Filesystem regions, commands requiring TiDB Cloud API keys, commands accepting only an FS token, delegated vault access, Preview limitations, platform dependencies, Drive9 companion dependency, and the macOS WebDAV/FUSE distinction.

`tdc-troubleshooting.md` covers actionable current failures, including missing or invalid API keys, missing FS tokens, ambiguous resource selection, missing/incompatible companion binary, quota/capacity errors, missing SQL users, mount timeout, macOS WebDAV/FUSE setup, and cleanup after interrupted operations.

Do not document pending telemetry commands or unimplemented serverless-function/package-manager commands.

## Navigation

Update both files:

```text
docs/pingcap-docs/docs/TOC-ai.md
docs/pingcap-docs/docs-cn/TOC-ai.md
```

Under Quick Start, Concepts, Guides, Examples, and Reference, add a nested `TiDB Cloud CLI (tdc) (Preview)` group with direct links to all corresponding tdc pages. Do not expose only the overview while leaving the other pages unreachable from the TOC.

Update both AI home pages:

```text
docs/pingcap-docs/docs/ai/_index.md
docs/pingcap-docs/docs-cn/ai/_index.md
```

Add a `TiDB Cloud CLI (tdc) (Preview)` section with concise grouped tables linking directly to every tdc page. `tdc-overview.md` is the tdc landing page; no generic `_index.md` is added under `ai/tdc/`.

## Existing Documentation Audit

The same work must update existing tdc project documents that no longer match the current implementation.

Current documents:

- Update `docs/priciples.md` so DB SQL credentials, Filesystem resource registry, FS tokens, profile/resource cardinality, Drive9 companion ownership, installation/update behavior, and command names match the current implementation.
- Update `docs/present.md` to remove `restore-git-workspace`, cover current Filesystem registry commands, use current credential paths, use valid vault mount authentication, and provide current cleanup commands.
- Audit `README.md` and `AGENTS.md` against the current binary and update any mismatches found.

Completed specs with historical or superseded behavior:

- `docs/spec/done/0002-local-config-and-credentials.md`
- `docs/spec/done/0004-api-client-auth-and-region-routing.md`
- `docs/spec/done/0009-tdc-fs-control-plane.md`
- `docs/spec/done/0010-tdc-fs-data-plane.md`
- `docs/spec/done/0011-tdc-fs-mount-runtime.md`
- `docs/spec/done/0011-ext01-fuse-cache-and-open-handle-correctness.md`
- `docs/spec/done/0013-github-actions-ci-cd.md`
- `docs/spec/done/0015-drive9-companion-wrapper-for-tdc-fs.md`

Preserve useful historical intent, but add clear supersession notices and remove statements presented as current behavior when they conflict with:

- `0015-drive9-companion-wrapper-for-tdc-fs.md`;
- `0016-profile-fs-resource-registry.md`;
- `0018-fs-token-auth-and-config-free-access.md`;
- the current installer/updater implementation;
- the current compiled command surface.

The native tdc FUSE implementation and low-level Git restore behavior are historical context, not current tdc behavior. The current implementation delegates the public Drive9 CLI surface to `tdc-drive9`.

Do not rewrite archived README snapshots or historical release notes. They describe the behavior of their named historical versions.

## Implementation Workflow

The two documentation directories are independent Git submodules:

1. Create an English docs branch from `release-8.5` in `docs/pingcap-docs/docs`.
2. Write and validate the English pages, `TOC-ai.md`, and AI home-page updates.
3. Create a Chinese docs branch from `release-8.5` in `docs/pingcap-docs/docs-cn`.
4. Translate the final English structure and update the Chinese TOC and AI home page.
5. Validate both submodule worktrees independently.
6. Commit changes inside each submodule.
7. Update the parent tdc repository gitlinks to the reviewed submodule commits.
8. Update the parent tdc current docs and completed-spec annotations in the same tdc change set.

Do not make the tdc build, tests, packaging, or release artifacts depend on either documentation submodule. The submodules are documentation source only.

## API Call Chain

This spec introduces no runtime API calls. Documentation examples describe existing TiDB Cloud and Drive9-backed command flows only.

No example may depend on an undocumented endpoint, raw server URL, or direct Drive9 invocation.

## Dependencies And Platform

- Depends on all completed MVP command specs through `0018`.
- Depends on the `pingcap/docs` and `pingcap/docs-cn` submodules tracking `release-8.5`.
- Adds no Go package, cgo, runtime, installer, or service dependency.
- Published installation instructions support release installers only.
- Platform documentation must be based on current tested behavior and must distinguish macOS WebDAV from optional macFUSE/FUSE.

## Acceptance Criteria

- Every English tdc page has a matching Chinese page at the same relative path.
- Every new Markdown basename starts with `tdc-` and is globally unique in its repository.
- Every page includes valid front matter and the standard Preview note.
- Both TOC files directly link every tdc page under the correct category.
- Both AI home pages directly link every tdc page.
- All internal links resolve to files or approved external URLs.
- The overview links feedback to `https://github.com/tidbcloud/tdc/issues`.
- Drive9 references link to `https://github.com/mem9-ai/drive9` and explain the bundled companion boundary.
- Quick Start reaches a successful DB or FS operation without introducing advanced features.
- Guides cover every current implemented command and every FS alias.
- No guide includes `tdc fs-git restore-git-workspace` or another absent command.
- No published page documents source compilation as an installation method.
- FS documentation states that macOS defaults to WebDAV and explains how installing macFUSE and explicitly selecting FUSE enables the complete mount experience.
- Examples use only synthetic values and never expose a real secret.
- `docs/priciples.md`, `docs/present.md`, README, AGENTS, and the listed completed specs no longer present superseded behavior as current behavior.
- Archived README snapshots and historical release notes remain unchanged.
- English and Chinese Markdown lint and link checks pass using each documentation repository's available validation workflow.
- The parent tdc repository records clean submodule gitlinks with no uncommitted documentation changes.

## Out Of Scope

- Implementing or changing tdc commands.
- Implementing telemetry, serverless functions, Homebrew, or Scoop.
- Documenting source builds as a supported user installation channel.
- Publishing documentation for pending, placeholder, internal-only, or direct Drive9 commands.
- Adding screenshots when text and verified command output are sufficient.
- Rewriting historical release notes or archived README snapshots.

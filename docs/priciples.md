# Agentic CLI \- tdc

CLI v2 for TiDB Cloud \| Init Date: 2026\-05\-18 \|@Todd Bao



## Problem Statement

## Target Users

## Success Metrics

## Scope \(MVP\)

## Success Criteria \& Exit

## Dependencies

## Milestones



## References 

### Why a New CLI?

1. `ticloud` has been stuck in beta for 3 years \(v1\.0\.0\-beta\.11 as of Dec 2025\)\. The name is generic, web search results show it is a brand for other businesses\. And, the command tree is inconsistent for predictibility\. 

2. For Starter tier, keep `ticloud` for backward compatibility\.

3. Clean\-sheet restart with a new name\. The ultimate goal is to ship GA at v1\.0 — no beta tag\. 

4. The new CLI is for **Starter** tier only from the v1\.0, and considers coherence with other tiers as well, so `--db-cluster-type` is required for `db` command\.

5. Naming guidelines: less than 5 chars, searchable \(avoid generic words like cloud/db/data\), ownable trademark**\.** Pre\-author 3\-5 candidates with trademark clearance before beta outreach\. Current WIP name is `tdc` \(**t**i**d**b**c**loud\)\.

6. TiDB Cloud public and private key pairs are the only credentials users need
   to provide initially\. tdc may generate and store derived credentials under
   `~/.tdc/credentials`, including DB SQL users and `tdc fs` resource API
   keys\.

### API Contract \& Backend Dependencies

1. Relies on the existing TiDB Cloud Starter \(formally Serverless\) API services, `tdc` does not necessarily mean API v2\.

2. `tdc db` \- existing TiDB Cloud Starter management, endpointed by region\. It depends on TiDB Cloud Starter API endpoints\.

3. `tdc fs` \- TiDB Cloud filesystem management and file operation, endpointed by region\. It depends on `tdc fs` API endpoints\.

4. `tdc configure` \- local config and credentials setup\. No dependencies\.

5. `tdc organization` \- TiDB Cloud account context management\. The confirmed
   MVP surface is project listing through TiDB Cloud IAM/account API
   endpoints. Organization list/describe stays an API gap until an endpoint is
   confirmed\.

6. Error mappings \- `tdc [ERROR]: <error message with actionable next step>`\.

7. API gaps handling \- requests to Starter and/or `tdc fs` engineering teams\. 

### Two Level Command Tree, and Two Level at Most

- Command pattern:** **`tdc noun|command [noun-function|subcommand]`

- Noun \(level 1 \- command\) maps to service domain\. Noun\-Action \(level 2 \- subcommand\) maps to domain verb\.

    - Level 1 \- command: `configure`, `db`, `fs`, `organization`

    - For `fs`, volume path is a special object, it does not count for the command or subcommand slot\.

- Muscle memory for developers and agents

    - `tdc help`

    - `tdc <command> help`

    - `tdc <command> <subcommand> help`

- Scriptable: every command same shape\.

    - Documentable: one page per `<command>`\. 

- Database tier\-agnostic, provides TiDB Cloud DBaaS as a whole instead of isolated offerings\. Tier/plan is just a flag, not baked into command tree\. Starts with Starter, ready for all tiers plug\-in in the future, for example: 

    - `tdc db create-db-cluster --db-cluster-name abc --db-cluster-type starter`

    - `tdc db create-db-cluster --db-cluster-name zyx --db-cluster-type premium`

- Examples:

    - `tdc configure` 

    - `tdc db create-db-cluster`

    - `tdc db create-db-cluster-branch`

    - `tdc db delete-db-cluster-branch`

    - `tdc fs create-file-system`

    - `tdc fs mount-file-system`

    - `tdc fs list-files`

    - `tdc fs copy-file`  



### Bias for Predictable, Self\-Explaination and Automation Friendly Behaviors \- [Agentic Friendly CLI](https://pingcap.feishu.cn/docx/P6PNdPXgpoASNYxJ2sXck5ssnRP)

1. **NO PROMPT** **for human input**\. The only exception is a human\-must initialization wizard: `tdc configure`\.

2. Use long options/flags only, no short options or aliases, e\.g: `--option` flag\. No `-o`; No `s` for `serverless`\. 

3. JSON output should be the default for successful **control plane** execution\. 

4. `--dry-run` — Mutating **control plane** operations can be validated before execution\.

5. `--no-*` Negation pattern for boolean flags with clear explicit declaration\.

### Telemetry

- Command\) and subcommand invoked

- Flags used \(names only — not values\)

- Error codes and execution time

- TiDB Cloud region, CLI version, OS type

- **NOT** credentials, file contents, or sensitive data

### Local Operation Logs

- Local operation logs are allowed for audit/debuggability and should be written under `~/.tdc/logs/`\.

- Logs record safe summaries only: command path, flag names, profile, region, duration, exit code, app error code/category, service, HTTP status, and request id\.

- Logs must not record flag values, SQL text/results, file contents, raw request/response bodies, connection strings, local paths, tdc fs raw paths, API keys, DB passwords, or tdc fs API keys\.

- Users and CI must be able to disable local operation logs with `TDC_LOGGING=off` or `[logging].enabled = false`\.

### Global Flags

1. `--profile` use which config/credentials, for FS it also decides the region\.

2. `--debug`

3. `--version` that is to make every level versionable, they can and might not return the same information about their version: e\.g: 

    1. `tdc --version`

    2. `tdc fs --version`, `tdc db --version`

    3. `tdc fs mount-file-system --version`

4. Install and update must be deterministic:

    - Install must support version pinning\.

    - No background or silent auto\-update\.

    - Update checks are explicit, for example `tdc update --check`\.

    - Self\-update is explicit, for example `tdc update --yes`, and must
      refuse package\-manager installs with actionable instructions\.

5. `--output`

6. `--query` / JMESPath** ** — Even with JSON, there is no way to extract a single field \(e\.g\., cluster ID from a create response\)\. The dev/agent must parse the entire response\. Provide `--query`  to remove noisy output and save tokens\. For `db` command, if SQL executions are designed in the future, it will use `--sql`, no conflicts here\.



### Open Issues

- ~~How get cluster connection string easily for stage 0?~~ Decided:
  `tdc db format-db-connection-string` after
  `tdc db create-db-sql-users`\.

- ~~What are the ~~~~`tdc db`~~~~ subcommands and TiDB Cloud Starter API stage 0 relies on?~~

- ~~Need the ~~~~`tidbcloud_fs`~~~~ schema structure to mock some example use cases\. ~~

- ~~Cost estimation\.~~

- Final CLI name\. The Followings are candidates:

    - ~~`tix`~~~~ \- A trade mark and a registered company\. The letters "tix" are a direct and common reference to the Totenkopf symbol\. This is an image of a human skull, often with crossed bones beneath it\. Super negative in Europe\.~~

    - ~~`tidb`~~~~ \- Too general in TiDB context, and it has multiple meanings\. ~~

    - ~~`tidbx`~~~~ \- TiDB X stands for the family of various plans on TiDB Cloud\.~~

    - ~~`tws`~~~~ \- TiDB Workspace\. A good umbrella for AI infra \(db \+ fs\), and feels close to dev, but less relevant to other plans\.~~

    - `tdc` \- TiDB Cloud initials, 3 letters, ntn\-style devoweling\. Clean, fast to type, no major collisions I can think of\. Best if the CLI stays cloud\-scoped\. Pairs well with a short install domain \(tdc\.dev if available\)\.





### Credentials Priority Order

|**Priority \(shortcut at first match\)**|**Credential Source**|**Description**|
|---|---|---|
|1 \(highest\)<br>|`--profile` flag in CLI\.<br>|Reads non\-sensitive data from `.tdc/config`<br>- `[<profile_name>]`<br>    - `region_code =` \(canonical, for example `aws-us-east-1`\)<br>Reads sensitive data from `.tdc/credentials`<br>- `[<profile_name>]`<br>    - `tdc_public_key =`<br>    - `tdc_private_key =`<br>    |
|2|No `--profile` in CLI, but environment variables exist\.<br>|Reads from envs\.<br>- `TDC_REGION_CODE`<br>- `TDC_PUBLIC_KEY`<br>- `TDC_PRIVATE_KEY`|
|3|No `--profile`, no environmental variables exist\.|Reads non\-sensitive data from `.tdc/config`<br>- `[default]`<br>    - `region_code =` \(canonical, for example `aws-us-east-1`\)<br>Reads sensitive data from `.tdc/credentials`<br>- `[default]`<br>    - `tdc_public_key =`<br>    - `tdc_private_key =`<br>    |

### Cloud Provider and Region Selection

Users choose one canonical region code. They do not provide separate cloud
provider fields, server URLs, filesystem metadata database URLs, or API
endpoints.

| Canonical Region Code | Cloud Provider | Region |
|---|---|---|
| `aws-us-east-1` | AWS | N. Virginia |
| `aws-us-west-2` | AWS | Oregon |
| `aws-eu-central-1` | AWS | Frankfurt |
| `aws-ap-northeast-1` | AWS | Tokyo |
| `aws-ap-southeast-1` | AWS | Singapore |
| `ali-ap-southeast-1` | Alibaba Cloud | Singapore |

The CLI internally parses the canonical `region_code` prefix to the correct
cloud provider and native region, then resolves TiDB Cloud Starter,
IAM/account, and `tdc fs` endpoints. This mapping is product logic and must not
require user-supplied server URLs.





### CLI Schema Mapping for TiDB Cloud FS

- Level 1 \- command: `fs`

    - control\-plane actions \(level 2 \- subcommand\): `create`, `delete`, `mount`, `umount`, `check`

    - data\-plane actions \(level 2 \- subcommand\): `cp`, `cat`, `ls`, `stat`, `mv`, `rm`, `mkdir`, `grep`, `find`

- General rules:

    - Reference filesystem concepts become `tdc fs` commands. Do not expose the
      reference implementation name in tdc user-facing output or APIs\.

    - All actions in `tdc fs <action>` with long flag parameters, no need to mimic well\-known commands' patterns, especially the control\-plane actions\.

- The MVP profile config file that the `tdc` CLI depends on is `~/.tdc/credentials` and `~/.tdc/config` for security sensitive and non\-sensitive data respectively with key\-value pairs under `[<profile_name>]` sections\.

    - `[default]` profile works with any `tdc` execution without `--profile`\. 

        ```Plain Text
        [default]
        tdc_public_key=<TIDB_CLOUD_ORG_PUBLIC_API_KEY>
        tdc_private_key=<TIDB_CLOUD_ORG_PRIVATE_API_KEY>
        
        [stage]
        tdc_public_key=<TIDB_CLOUD_ORG_PUBLIC_API_KEY>
        tdc_private_key=<TIDB_CLOUD_ORG_PRIVATE_API_KEY>
        
        ```



### How `tdc` CLI bootstraps?

1. Install `tdc` by running single command\.

2. **`tdc configure [--profile <profile_name>]` prompts for user interactions, creates `~/.tdc/credentials`, and `~/.tdc/config`:**

    1. `[<profile_name>]`, default profile if `--profile` is omitted\.

    2. Prompt for the TiDB Cloud API Public KEY \- `tdc_public_key` \(Skip to omit\)\.

    3. Prompt for the TIDB Cloud API Private KEY \- `tdc_private_key` \(Skip to omit\)\.

    4. Prompt for canonical region code \- `region_code` \(for example `aws-us-east-1` or `ali-ap-southeast-1`\)\.

    6. Do not prompt for server URLs, API endpoints, or filesystem metadata database URLs.



### DB Users Entry Command

- **Create a new DB**

    - Run `tdc db create-db-cluster --db-cluster-name <cluster_name> --db-cluster-type starter [--profile <profile_name>] [--root-password <root_password>] [and all the other parameters]`

        - Purpose: Create a new TiDB Cloud Starter cluster\.

        - Prerequisites:

            - Have access to organization owner's public/private API keys\.

        - Command workflow actions:

            - Create a TiDB Cloud Starter cluster with the specified root password\.

            - Return message\.

- **Prepare DB SQL query users**

    - Run `tdc db create-db-sql-users --db-cluster-id <cluster_id> [--profile <profile_name>]`

        - Purpose: Create and persist tdc-managed SQL credentials for later
          `tdc db execute-sql-statement` commands\.

        - Prerequisites:

            - Have TiDB Cloud public/private API keys with permission to manage
              SQL users on the target Starter cluster\.

        - Command workflow actions:

            - Create three SQL users if they do not already exist:
              read-only \(`role_readonly`\), read-write \(`role_readwrite`\),
              and admin \(`role_admin`\)\.

            - The command is idempotent. Running it again must reuse or repair
              the same tdc-managed users, not create another group\.

            - Store generated usernames and passwords in `~/.tdc/credentials`
              under the active profile and cluster ID\.

        - Return structured JSON by default\.

- **Create DB connection string**

    - Run `tdc db format-db-connection-string --db-cluster-id <cluster_id> [--read-write | --read-only | --admin] [--format mysql-uri|jdbc|go-sql-driver|sqlalchemy|env] [--profile <profile_name>]`

        - Purpose: Print a connection string or dotenv component variables
          from tdc-managed SQL credentials prepared by
          `tdc db create-db-sql-users`\.

        - Access mode:

            - Default is read-write \(`role_readwrite`\)\.

            - `--read-write` explicitly uses the read-write user\.

            - `--read-only` explicitly uses the read-only user\.

            - `--admin` explicitly uses the admin user\.

            - Do not infer access mode from SQL text or command context\. There
              is no auto mode\.

        - Output formats:

            - `mysql-uri` is the default\.

            - `jdbc`, `go-sql-driver`, and `sqlalchemy` support common
              ecosystem formats\.

            - `env` outputs `.env` component variables such as host, port,
              user, password, database, SSL mode, and access mode so agents can
              assemble framework-specific connection strings themselves\.

        - Do not store generated connection strings in config, credentials,
          logs, or telemetry\.

- **Run one SQL statement**

    - Run `tdc db execute-sql-statement --db-cluster-id <cluster_id> --sql <sql_string> [--profile <profile_name>]`

        - Purpose: Execute a single SQL statement against a Starter cluster or
          branch\.

        - Prerequisites:

            - `tdc db create-db-sql-users` has prepared local SQL credentials for the
              target cluster\.

        - Access mode:

            - Default is read-write \(`role_readwrite`\)\.

            - `--read-write` explicitly uses the read-write user\.

            - `--read-only` explicitly uses the read-only user\.

            - `--admin` explicitly uses the admin user\.

            - Do not infer access mode from SQL text. There is no auto mode\.

        - Transport:

            - HTTPS SQL API execution is preferred and uses the TiDB Cloud
              Serverless SQL API shape\.

            - MySQL one-shot execution is an explicit fallback transport, not a
              hidden automatic retry for write-capable statements\.



### Detailed `tdc fs create-file-system` Schema Draft

- `--file-system-name`: The tdc fs resource name\.

- `[--profile]`: The profile for reading the TiDB Cloud API key and canonical
  `region_code`\.

- `[--dry-run]`: Validate profile, credentials, provider/region support,
  permission, and request construction without mutating remote resources\.

- The command must not accept a server URL, API endpoint, or filesystem metadata
  database URL from the user\.

- The CLI resolves the required backend endpoint internally from canonical
  `region_code`, then provisions or initializes the
  filesystem through the supported TiDB Cloud and `tdc fs` APIs\.

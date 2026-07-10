# tdc 演示脚本

这个文档用于现场展示 tdc 的 MVP 能力。演示故事线是：一个 agent 需要在 TiDB Cloud Starter 上创建数据库集群，准备不同权限的 SQL 访问账号，创建一个 tdc fs workspace，把代码和任务文件放进去，通过 mount 和 data plane 两种方式互相读写，再用 git/journal/vault 记录和保护 agent workflow。

所有命令都使用本地构建出来的二进制路径 `bin/tdc`。

## 演示前准备

先确认二进制可用：

```bash
bin/tdc --version
bin/tdc help
```

配置默认 profile。现场手动输入 key 时使用交互式配置：

```bash
bin/tdc configure
```

CI 或提前准备时可以用非交互式配置：

```bash
TDC_REGION_CODE=aws-us-east-1 \
TDC_PUBLIC_KEY="$TDC_PUBLIC_KEY" \
TDC_PRIVATE_KEY="$TDC_PRIVATE_KEY" \
bin/tdc configure --non-interactive
```

如果现场需要解析 JSON，建议准备 `jq`：

```bash
jq --version
```

## 1. Organization：找到项目

先展示 tdc 可以通过 TiDB Cloud API key 读取当前账号可见的项目。

```bash
bin/tdc organization list-projects --output text
```

取一个 project id，后续创建 Starter cluster 会用到：

```bash
export PROJECT_ID="$(bin/tdc organization list-projects | jq -r '.projects[0].id')"
echo "$PROJECT_ID"
```

可以说明：`organization` 是只读 control plane，适合脚本和 agent 先发现当前账号可操作的 TiDB Cloud 项目。

## 2. DB Control Plane：Starter 集群增删改查

生成一个本次演示用的资源名前缀：

```bash
export DEMO_ID="$(date +%Y%m%d%H%M%S)"
export CLUSTER_NAME="tdc-demo-${DEMO_ID}"
```

先用 dry-run 展示请求会做什么，但不真的创建资源：

```bash
bin/tdc db create-db-cluster \
  --db-cluster-name "$CLUSTER_NAME" \
  --db-cluster-type starter \
  --project-id "$PROJECT_ID" \
  --dry-run
```

正式创建一个 Starter cluster：

```bash
bin/tdc db create-db-cluster \
  --db-cluster-name "$CLUSTER_NAME" \
  --db-cluster-type starter \
  --project-id "$PROJECT_ID"
```

查询列表并取出刚创建的 cluster id：

```bash
bin/tdc db list-db-clusters --output text
export CLUSTER_ID="$(bin/tdc db list-db-clusters | jq -r --arg name "$CLUSTER_NAME" '.clusters[] | select(.display_name == $name) | .id' | head -n 1)"
echo "$CLUSTER_ID"
```

查看单个 cluster 详情。等待 `state` 变成 `ACTIVE` 后再继续 SQL 演示：

```bash
bin/tdc db describe-db-cluster --db-cluster-id "$CLUSTER_ID" --output text
bin/tdc db describe-db-cluster --db-cluster-id "$CLUSTER_ID" --query state
```

更新 cluster 名称，展示 update：

```bash
export CLUSTER_RENAMED="${CLUSTER_NAME}-renamed"

bin/tdc db update-db-cluster \
  --db-cluster-id "$CLUSTER_ID" \
  --db-cluster-name "$CLUSTER_RENAMED"

bin/tdc db describe-db-cluster --db-cluster-id "$CLUSTER_ID" --output text
```

## 3. DB SQL：准备账号、连接串、不同 SQL roles

tdc 不直接复用 TiDB Cloud API key 执行 SQL，而是为这个 cluster 准备三类稳定 SQL 用户：`read_only`、`read_write`、`admin`。这个命令可重入，重复执行不会创建新的一组用户。

```bash
bin/tdc db create-db-sql-users --db-cluster-id "$CLUSTER_ID"
```

输出不同角色的连接串。默认是 read-write，也可以显式指定：

```bash
bin/tdc db format-db-connection-string --db-cluster-id "$CLUSTER_ID" --read-write --format mysql-uri
bin/tdc db format-db-connection-string --db-cluster-id "$CLUSTER_ID" --read-only --format env
bin/tdc db format-db-connection-string --db-cluster-id "$CLUSTER_ID" --admin --format jdbc
```

使用 admin 做 DDL：

```bash
bin/tdc db execute-sql-statement \
  --db-cluster-id "$CLUSTER_ID" \
  --admin \
  --sql "create database if not exists tdc_demo"

bin/tdc db execute-sql-statement \
  --db-cluster-id "$CLUSTER_ID" \
  --admin \
  --sql "create table if not exists tdc_demo.messages (id int primary key, body varchar(128), created_at timestamp default current_timestamp)"
```

使用 read-write 写入数据：

```bash
bin/tdc db execute-sql-statement \
  --db-cluster-id "$CLUSTER_ID" \
  --read-write \
  --sql "insert into tdc_demo.messages (id, body) values (1, 'hello from tdc') on duplicate key update body = values(body)"
```

使用 read-only 查询数据：

```bash
bin/tdc db execute-sql-statement \
  --db-cluster-id "$CLUSTER_ID" \
  --read-only \
  --sql "select id, body, created_at from tdc_demo.messages" \
  --output text
```

使用 admin 查看权限信息：

```bash
bin/tdc db execute-sql-statement \
  --db-cluster-id "$CLUSTER_ID" \
  --admin \
  --sql "show grants" \
  --output text
```

可以说明：`execute-sql-statement` 默认使用 HTTPS SQL API，一次命令执行一个 SQL statement，不保持长连接；需要兼容性 fallback 时可以显式使用 `--transport mysql`。

## 4. tdc fs Control Plane：创建、检查、删除 workspace resource

先检查默认 profile 的 fs 状态。还没创建 fs resource 时，远端检查会给 warning：

```bash
bin/tdc fs check-file-system --output text
```

预览创建：

```bash
export FS_NAME="tdc-demo-fs-${DEMO_ID}"

bin/tdc fs create-file-system \
  --file-system-name "$FS_NAME" \
  --dry-run
```

正式创建 tdc fs resource：

```bash
bin/tdc fs create-file-system \
  --file-system-name "$FS_NAME"
```

再次检查：

```bash
bin/tdc fs check-file-system --output text
```

可以说明：tdc fs control plane 当前有 `create-file-system`、`check-file-system`、`delete-file-system`。它没有独立的 `update-file-system`，因为 resource name、region、tenant 这类元数据是 provision-time 属性；如果要换资源，使用 delete + create 更确定。

## 5. tdc fs Data Plane：直接操作远端文件

创建目录、上传、读取、列目录、查看 metadata：

```bash
bin/tdc fs create-directory --path /demo --mode 0755

printf 'hello from data plane\n' | bin/tdc fs copy-file \
  --from-stdin \
  --to-remote /demo/from-data-plane.txt \
  --tag source=data-plane \
  --description "created through tdc fs data-plane"

bin/tdc fs list-files --path /demo --output text
bin/tdc fs read-file --path /demo/from-data-plane.txt
bin/tdc fs describe-file --path /demo/from-data-plane.txt --output text
```

展示常见 Linux 风格 alias。alias 只缩短命令名，flags 仍然保持长名称：

```bash
bin/tdc fs ls --path /demo --output text
bin/tdc fs cat --path /demo/from-data-plane.txt
```

展示搜索和查找：

```bash
bin/tdc fs search-file-content --path /demo --pattern "data plane" --output text
bin/tdc fs find-files --path /demo --file-name-pattern "*.txt" --output text
```

## 6. tdc fs Mount：像本地文件系统一样操作，并验证和 data plane 互通

准备 mount 目录，并用默认 driver 挂载。默认是 `auto`，会优先尝试 FUSE；如果要强制 FUSE，可以加 `--driver fuse`。

```bash
export MOUNT_PATH="/tmp/tdc-demo-${DEMO_ID}"
mkdir -p "$MOUNT_PATH"

bin/tdc fs mount-file-system \
  --file-system-name "$FS_NAME" \
  --mount-path "$MOUNT_PATH" \
  --driver fuse
```

先通过 mount 读取刚才由 data plane 写入的文件，证明 data plane 写入对 mount 可见：

```bash
cat "$MOUNT_PATH/demo/from-data-plane.txt"
```

再通过 mount 写入一个文件：

```bash
printf 'hello from mounted filesystem\n' > "$MOUNT_PATH/demo/from-mount.txt"
ls -la "$MOUNT_PATH/demo"
```

通过 data plane 读取 mount 写入的文件，证明 mount 写入对 data plane 可见：

```bash
bin/tdc fs read-file --path /demo/from-mount.txt
bin/tdc fs list-files --path /demo --output text
```

展示 drain：不卸载，只要求 FUSE runtime flush dirty state。

```bash
bin/tdc fs drain-file-system --mount-path "$MOUNT_PATH" --output text
```

## 7. tdc fs-git：在 tdc fs mount 里做 Git-aware workspace

Git workspace 需要目标路径位于 tdc fs mount 下，因为它会把本地 `.git` 状态、远端 tree manifest、overlay 写入都绑定到这个 workspace。

```bash
mkdir -p "$MOUNT_PATH/repos"

bin/tdc fs-git clone-git-workspace \
  --repo-url https://github.com/octocat/Hello-World.git \
  --target-path "$MOUNT_PATH/repos/hello" \
  --blobless \
  --hydrate background
```

进入这个 workspace，可以继续用普通 git 命令：

```bash
git -C "$MOUNT_PATH/repos/hello" status --short
printf '\nhello from tdc fs-git demo\n' >> "$MOUNT_PATH/repos/hello/README"
git -C "$MOUNT_PATH/repos/hello" status --short
```

展示 tdc 的 Git restore/hydrate 入口：

```bash
bin/tdc fs-git hydrate-git-workspace --target-path "$MOUNT_PATH/repos/hello"
bin/tdc fs-git restore-git-workspace --target-path "$MOUNT_PATH/repos/hello"
```

如果要展示 linked worktree：

```bash
bin/tdc fs-git add-git-worktree \
  --base-path "$MOUNT_PATH/repos/hello" \
  --worktree-path "$MOUNT_PATH/repos/hello-feature" \
  --branch-name demo-feature

git -C "$MOUNT_PATH/repos/hello-feature" status --short

bin/tdc fs-git remove-git-worktree \
  --worktree-path "$MOUNT_PATH/repos/hello-feature" \
  --force
```

可以说明：tdc fs-git 不是替代 git，而是在 tdc fs mount 里提供 fast clone、hydrate、restore、worktree 这类 agent workspace 加速路径。

## 8. tdc fs-journal：记录 agent/workflow 事件

创建一个 journal：

```bash
export JOURNAL_ID="jrn-demo-${DEMO_ID}"

bin/tdc fs-journal create-journal \
  --journal-id "$JOURNAL_ID" \
  --journal-kind agent \
  --title "tdc demo run ${DEMO_ID}" \
  --actor "agent:demo" \
  --label demo=present
```

追加事件。可以从 flag 传 JSON，也可以从 stdin 传 JSONL：

```bash
bin/tdc fs-journal append-journal-entries \
  --journal-id "$JOURNAL_ID" \
  --entry-json '{"type":"demo.started","summary":{"message":"presentation started"}}' \
  --entry-json '{"type":"db.ready","summary":{"cluster":"ready for sql"}}'

printf '%s\n' \
  '{"type":"fs.mounted","summary":{"path":"mounted and visible"}}' \
  '{"type":"demo.completed","summary":{"message":"presentation completed"}}' \
  | bin/tdc fs-journal append-journal-entries \
      --journal-id "$JOURNAL_ID"
```

读取、搜索、验证 journal：

```bash
bin/tdc fs-journal read-journal-entries --journal-id "$JOURNAL_ID" --output text
bin/tdc fs-journal search-journal-entries --entry-type fs.mounted --include-entries
bin/tdc fs-journal verify-journal --journal-id "$JOURNAL_ID" --output text
```

可以说明：journal 是 append-only 的 agent/workflow ledger，用来记录任务轨迹、审计事件和可验证执行历史，不是普通文本日志文件。

## 9. tdc fs-vault：管理 secret、授权 agent、注入环境变量

创建一个 secret。`key=value` 直接传值，`key=@file` 从文件读取，`key=-` 从 stdin 读取：

```bash
printf 'demo-token-%s\n' "$DEMO_ID" > /tmp/tdc-demo-token.txt

bin/tdc fs-vault create-secret \
  --secret-name demo-db \
  --field DB_URL="mysql://example.invalid/demo" \
  --field API_TOKEN=@/tmp/tdc-demo-token.txt
```

读取整个 secret 或某个字段：

```bash
bin/tdc fs-vault read-secret --secret-name demo-db
bin/tdc fs-vault read-secret --secret-name demo-db --field DB_URL --format raw
bin/tdc fs-vault read-secret --secret-name demo-db --format env
```

创建一个只读 delegated grant 给 agent，只允许读 `demo-db/DB_URL`：

```bash
export TDC_VAULT_TOKEN="$(bin/tdc fs-vault create-grant \
  --agent-id demo-agent \
  --scope demo-db/DB_URL \
  --permission read \
  --ttl 1h \
  --token-only)"

echo "$TDC_VAULT_TOKEN"
```

用 delegated token 读取被授权字段：

```bash
bin/tdc fs-vault read-secret \
  --secret-name demo-db \
  --field DB_URL \
  --format raw \
  --vault-token "$TDC_VAULT_TOKEN"
```

把 secret 注入子进程环境变量：

```bash
bin/tdc fs-vault run-with-secret \
  --secret-path /n/vault/demo-db \
  -- env | grep -E '^(DB_URL|API_TOKEN)='
```

查看 audit：

```bash
bin/tdc fs-vault list-audit-events --secret-name demo-db --limit 20 --output text
```

如果现场环境支持 FUSE，也可以把 vault 只读挂载出来：

```bash
export VAULT_MOUNT_PATH="/tmp/tdc-demo-vault-${DEMO_ID}"
mkdir -p "$VAULT_MOUNT_PATH"

bin/tdc fs-vault mount-vault --mount-path "$VAULT_MOUNT_PATH"
find "$VAULT_MOUNT_PATH" -maxdepth 2 -type f -print
cat "$VAULT_MOUNT_PATH/demo-db/DB_URL"
bin/tdc fs-vault unmount-vault --mount-path "$VAULT_MOUNT_PATH"
```

可以说明：vault 是给 agent 使用的 secret surface，重点是 scoped grant、审计、只读挂载和安全地注入子进程，而不是把 secret 当普通文件随意复制。

## 10. 清理资源

先卸载 tdc fs mount：

```bash
bin/tdc fs drain-file-system --mount-path "$MOUNT_PATH" --output text
bin/tdc fs unmount-file-system --mount-path "$MOUNT_PATH" --ignore-absent
rm -rf "$MOUNT_PATH"
```

删除 vault secret 和本地临时 token 文件：

```bash
bin/tdc fs-vault delete-secret --secret-name demo-db
rm -f /tmp/tdc-demo-token.txt
```

删除 tdc fs resource。这个命令会删除远端 tdc fs resource，并清理默认 profile 下保存的 `fs_*` metadata 和 `fs_api_key`：

```bash
bin/tdc fs delete-file-system \
  --file-system-name "$FS_NAME" \
  --confirm-file-system-name "$FS_NAME"
```

删除 Starter cluster。删除是非交互式的，tdc 会先读取远端 cluster，再按 ID 删除：

```bash
bin/tdc db delete-db-cluster \
  --db-cluster-id "$CLUSTER_ID"
```

确认 cluster 已删除：

```bash
bin/tdc db describe-db-cluster --db-cluster-id "$CLUSTER_ID"
```

最后可以删除本地 tdc 配置；如果这台机器还要继续使用 tdc，就不要删：

```bash
# rm -rf ~/.tdc
```

## 现场讲解重点

- tdc 是 agent-friendly CLI：长命令、长 flags、JSON 默认输出、`--query`、`--output text`、`--dry-run`，适合人和 agent 同时使用。
- TiDB Cloud control plane 和 SQL execution 是分离的：TiDB Cloud API key 用于管理资源，SQL 使用 tdc 准备好的 read-only/read-write/admin SQL 用户。
- tdc fs 有两种访问面：data plane 命令适合脚本，mount 适合把远端 workspace 变成本地文件系统；两者对同一个远端 namespace 可见。
- git、journal、vault 虽然都依赖 tdc fs resource 和 `fs_api_key`，但它们是顶层命令，因为它们分别代表 Git workspace、append-only workflow ledger、secret management 三个独立产品域。
- 清理资源时先 unmount，再删 vault secret 和 fs resource，最后删 DB cluster，避免遗留免费 Starter 资源。

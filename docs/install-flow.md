# my-pi-package 安装流程

本文描述 Go 版 CLI **my-pi-package** 的安装与相关命令行为（实现入口：`cmd/my-pi-package`）。

## 1. 它是什么

**my-pi-package** 是一个有主见的 [Pi](https://github.com/earendil-works/pi-mono) coding agent 扩展安装器：

1. 确保本机有 `pi` 命令（缺失时可 `npm install -g`）；
2. 按 YAML catalog 选择并安装扩展；
3. 持久化写入 Pi 的 `settings.json`（以及 Compound 的特殊目录）；
4. 自身不长期驻留——通过发布产物或本地编译得到的二进制执行即可。

**核心原则：**

| 原则 | 含义 |
| --- | --- |
| Lazy by default | 默认安装完整 catalog |
| Idempotent | 已安装则跳过，可反复执行 |
| Non-destructive | 写 settings 前备份 |
| Selective | 交互 picker / `--only` / `--except` |
| YAML catalog | 插件与版本在根目录 `catalog.yaml` 中维护 |

## 2. 前置条件

| 依赖 | 必须 | 用途 |
| --- | --- | --- |
| **my-pi-package 二进制** | 是 | 本 CLI |
| **npm** | 是 | 全局安装 Pi |
| **`pi` CLI** | 是（可自动装） | `pi install` / `remove` / `update` |
| **git** | 部分包 | `git:` 源 |
| **Bun** | 可选 | Compound Engineering |

```bash
my-pi-package doctor
```

## 3. 命令

```bash
my-pi-package [command] [options]
```

| Command | 默认 | 作用 |
| --- | --- | --- |
| `install` | ✅ | 安装 catalog |
| `status` | | 已装 / legacy / 缺失 / 目录外包 |
| `update` | | 刷新 Compound（若已装）+ `pi update` |
| `remove` | | 按 id 或 raw source 卸载 |
| `doctor` | | 环境健康检查 |
| `skills` | | 从 marketplace 安装 / 删除 Pi agent skills |

**install 选项：**

| 选项 | 含义 |
| --- | --- |
| `-y` / `--yes` | 跳过 TUI |
| `--only <list>` | 仅 category 或 id |
| `--except <list>` | 排除 |
| `-l` / `--local` | 项目级 `.pi/settings.json` |
| `--catalog <path>` | 自定义 YAML |
| `-h` / `--help` | 帮助 |

```bash
my-pi-package
my-pi-package --yes
my-pi-package --only core
my-pi-package --only subagents,mcp --local
my-pi-package status
my-pi-package update
my-pi-package remove subagents
my-pi-package doctor
```

## 4. install 主流程

```
parseArgs → 默认 install
  → Load catalog (embed 或 --catalog)
  → ResolveSelection + ExpandDependsOn
  → [TTY 且无 --yes/--only/--except] bubbletea picker
  → EnsurePi
  → 读 settings sources → toInstall / already / legacy
  → subagent overrides（若含 subagents）
  → NormalizeLoadOrder
  → 逐个安装：
        compound → bunx …
        其他 → legacy remove + pi install <source>
  → NormalizeLoadOrder
  → cheatsheet + auth 提示
```

### Source 解析（YAML version）

见 [catalog-yaml.md](./catalog-yaml.md)。摘要：

- `version: latest` → `npm:pkg` / `git:repo`
- 锁定版本 → `npm:pkg@1.2.3` / `git:repo@ref`
- `type: compound` → `bunx package@version`，不走 `pi install`

### settings 路径

| 模式 | 路径 |
| --- | --- |
| 全局 | `~/.pi/agent/settings.json`（或 `PI_CODING_AGENT_DIR`） |
| `--local` | `<cwd>/.pi/settings.json` |

修改前备份：`settings.json.my-pi-package.<timestamp>.bak`。

### 退出码

| Code | 含义 |
| --- | --- |
| 0 | 成功 / 无事可做 |
| 1 | 部分失败 / doctor 问题 |
| 2 | 参数或 settings 非法 |
| 127 | Pi 不可用 |

## 5. 其他命令

- **status**：catalog 已装 / legacy / 缺失 / 非 catalog 包  
- **update**：若含 compound 则强制刷新 CE，再 `pi update`  
- **remove**：`my-pi-package remove <id|source>`  
- **doctor**：Node/npm/git/bun、pi、settings、load-order、Compound、auth  

## 5b. skills — marketplace / plugin skills

将 Claude/Codex 风格仓库里的 **skill 目录** 拷贝到 Pi 可加载路径。

| 范围 | 路径 |
| --- | --- |
| 全局 | `~/.pi/agent/skills`（或 `$PI_CODING_AGENT_DIR/skills`） |
| `-l` / `--local` | `./.pi/skills` |

```bash
my-pi-package skills install ipfans/demo-plugins   # owner/repo → github clone
my-pi-package skills install mattpocock/skills    # plugin.json 单插件仓库
my-pi-package skills install ./local/marketplace
my-pi-package skills remove
```

**Manifest 优先级（先找到的生效）：**

1. `.claude-plugin/plugin.json` — 单插件；读取 `skills` 路径列表  
2. `.claude-plugin/marketplace.json`  
3. `.agents/plugins/marketplace.json`  

存在有效 `plugin.json` 时**不再**读取 marketplace。

**install 流程：**

1. 解析 source：本地目录 / `owner/repo` → `https://github.com/owner/repo.git` / git URL  
2. 远程则 `git clone --depth 1` 到临时目录  
3. 按优先级加载 manifest  
4. **plugin.json 模式**  
   - 解析 `skills[]` 相对路径（如 `./skills/engineering/ask-matt`），校验在仓库根下且为目录  
   - skill 名为路径 basename（`ask-matt`）；**整夹拷贝**  
   - TTY 下 TUI **多选 skills**（默认全选）；`-y` 需 `--all` 或 `--only skill1,skill2`  
5. **marketplace 模式**  
   - 解析每个 plugin 的 `source`（字符串路径或 object 的 `path` / 相对 `url`）  
   - 仅展示含 `skills/` 子目录的插件  
   - TTY 下 TUI **多选插件**（默认全选）；`-y` 需 `--all` 或 `--only plugin1,plugin2`  
   - 将所选插件 `skills/*` 目录拷贝到目标路径  
6. 目标路径下 skill 目录**已存在则覆盖**

**remove 流程：**

1. 列出目标目录下已安装的 skill 文件夹  
2. TTY 下 TUI 多选（默认不选，至少选 1）；`-y` 需 `--only skill1,skill2`  
3. `RemoveAll` 所选目录  

不依赖 catalog；不记录 provenance。

## 6. 开发与发布

见根目录 `Taskfile.yml` 与 [go-architecture.md](./go-architecture.md) 的发布章节。

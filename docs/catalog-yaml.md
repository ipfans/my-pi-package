# YAML 插件目录设计

## 目标

1. **单一真相源**：可安装插件在 YAML 中声明，增删改不必改 Go 代码。
2. **版本可控**：锁定版本，或 `latest` 跟踪最新。
3. **可嵌入发布**：根目录 `catalog.yaml` 经 `//go:embed` 打进二进制；可用 `--catalog path` 覆盖。

## 文件

```
catalog.yaml          # 项目根目录 — 日常只改这个文件
catalogdata.go        # package catalogdata；//go:embed catalog.yaml 打进二进制
```

编辑 `catalog.yaml` 后重新 `task build` / 发布即可；无需改 Go 代码。

仍可用运行时覆盖：

```bash
my-pi-package --catalog ./my-catalog.yaml install
```

## Schema

```yaml
defaults:
  version: latest

pi:
  package: "@earendil-works/pi-coding-agent"
  version: latest

categories:
  - core
  - ui
  - research
  - frameworks
  - themes

load_first:
  - extension-settings

packages:
  - id: subagents
    category: core
    type: npm                     # npm | git | compound
    package: pi-subagents
    version: latest               # latest | semver | git commit/tag
    description: "Sub-agent execution"
    hint: "Run isolated sub-agents for parallel work."
    # legacy_sources: []
    # depends_on: []

  - id: memory
    category: core
    type: npm
    package: pi-memory-md
    version: latest
    legacy_sources:
      - git:github.com/VandeeFeng/pi-memory-md
    description: "Markdown-backed memory"
    hint: "Persistent memory stored as Markdown files."

  - id: todos
    category: ui
    type: npm
    package: "@tintinweb/pi-tasks"
    version: latest
    legacy_sources:
      - git:github.com/tintinweb/pi-manage-todo-list@b75c449aa85ce328e9a8b632f62bf642aed40359
    description: "Todo list management"
    hint: "…"

  - id: compound
    category: frameworks
    type: compound
    package: "@every-env/compound-plugin"
    version: "3.0.0"
    plugin_name: compound-engineering
    depends_on:
      - subagents
      - pi-ask-user
    description: "Official Compound Engineering"
    hint: "Requires Bun; skipped if missing."
```

## 版本解析

| type | version | 结果 |
| --- | --- | --- |
| npm | latest / 空 | `npm:<package>` |
| npm | 具体版本 | `npm:<package>@<version>` |
| git | latest / 空 | `git:<repo>` |
| git | ref | `git:<repo>@<version>` |
| compound | 任意 | 展示 source 可为 `npm:…`；安装走 `bunx <package>@<version>` |

```go
// version "latest" 不写 pin；否则追加 @version
```

## 运维

| 场景 | 操作 |
| --- | --- |
| 加插件 | 编辑 `packages` |
| 锁版本 | `version: "1.2.3"` 或 git commit |
| 跟最新 | `version: latest` |
| 自定义目录 | `my-pi-package --catalog ./my-catalog.yaml install` |

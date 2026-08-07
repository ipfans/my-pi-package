# my-pi-package 架构

## 目标

Go CLI，为 Pi coding agent 提供 YAML 驱动的扩展安装：

- 流程见 [install-flow.md](./install-flow.md)
- 插件目录见 [catalog-yaml.md](./catalog-yaml.md)
- 交互 UI：`charm.land/bubbletea/v2`
- 非交互路径不启动 TUI

## 包布局

```
my-pi-package/
├── go.mod
├── catalog.yaml             # 插件真相源（根目录，方便直接改）
├── catalogdata.go           # package catalogdata：embed 根目录 catalog.yaml
├── Taskfile.yml
├── .goreleaser.yaml
├── .github/workflows/
├── docs/
├── catalog/                 # 解析 / 校验 / Source 解析
│   ├── catalog.go
│   └── catalog_test.go
├── settings/
├── pi/
├── install/
├── skills/                  # marketplace skills install / remove
├── tui/                     # catalog picker + generic multi-select
└── cmd/my-pi-package/
    ├── main.go
    └── skills.go            # skills subcommand wiring
```

`main` 只做接线；业务在 domain 包中。

## 依赖

| 库 | 用途 |
| --- | --- |
| `charm.land/bubbletea/v2` | 交互安装 TUI |
| `gopkg.in/yaml.v3` | catalog YAML |
| 标准库 | `os/exec`、`encoding/json` |

## 本地开发（Task）

```bash
task              # 默认：列出任务
task build        # 编译到 ./bin/my-pi-package
task test         # go test ./...
task run -- status
task tidy
task release:snapshot  # 本地 GoReleaser 快照
```

## 发布

- **GoReleaser** 配置：`.goreleaser.yaml`
- **GitHub Actions**：push 匹配 `v*` 的 tag 时构建多平台二进制并创建 GitHub Release
- 二进制名：`my-pi-package`

```bash
git tag v0.1.0
git push origin v0.1.0
```

## TUI

仅当 stdin/stdout 均为 TTY 且未使用 `--yes` / `--only` / `--except`：

1. lazy-or-pick
2. 分组 multiselect
3. 退出后在 TUI 外 inherit 运行 `pi install` 等子进程

## 测试

- catalog：解析、Source、depends_on（表驱动）
- 不默认 spawn 真 pi；可选集成用环境变量扩展

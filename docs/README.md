# Design Docs

本目录存放 **my-pi-package** 的设计与实现文档。

| 文档 | 内容 |
| --- | --- |
| [install-flow.md](./install-flow.md) | CLI 安装与命令流程 |
| [catalog-yaml.md](./catalog-yaml.md) | YAML 插件目录：字段、版本锁定 / `latest` |
| [go-architecture.md](./go-architecture.md) | Go 包布局、bubbletea TUI、发布 |

## 约定

- **所有设计文档**写在仓库根目录 `docs/` 下。
- 插件清单以 YAML 为唯一真相源（根目录 `catalog.yaml`），代码只负责解析与安装编排。
- 二进制名与模块名均为 **my-pi-package**。

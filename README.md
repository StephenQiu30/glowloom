# 浮光

浮光围绕项目与剧本到镜头生成和素材管理组织创作：项目、画布、镜头、故事板、设定、素材，以及账号与管理功能。正式前端采用当前设计稿的页面实现；功能和数据暂用 mock，真实业务按正式原始需求逐项接入和验收。

## 当前设计入口

- [功能需求总表](docs/requirement/01-功能需求总表.md)
- [数据库设计](docs/design/浮光页面数据设计.md)
- [接口设计](docs/design/浮光页面接口设计.md)
- [当前PRD](docs/prd/01-产品需求文档.md)
- [验收标准](docs/requirement/02-非功能需求规格.md)

[浮光Figma](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/) 是视觉依据；正式首页为 `/`，代码位于 `frontend/src/app/`。旧业务页面、API、数据库结构与工作台不约束当前需求。历史说明隔离在 `docs/history/`，只供单独安排的迁移或旧环境维护查阅，不作为当前需求来源。

## 本地阅读

从 [docs/README.md](docs/README.md) 阅读当前文档，直接维护 `docs/` 中的 Markdown，并用 Git 记录修订。Obsidian 打开本仓库目录作为 Vault，使用同一批文件；模板、相对链接与图谱设置继续保留。详细用法见 [文档与 Obsidian 说明](docs/KNOWLEDGE.md)。

前端开发服务可在 `frontend` 使用 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`，访问 `/`；启动不代表真实业务接入。旧前端实现已清理；现有数据库与文件数据没有删除或迁移。

协作遵循 [AGENTS](AGENTS.md)，工程工具与门禁见 [PROJECT](PROJECT.md)，当前事项见 [BACKLOG](BACKLOG.md)。第三方许可保持原文，见 [许可](docs/licenses/index.md)。

正式前端替换与浏览器证据见[替换验收](docs/test/浮光正式前端替换验收.md)。

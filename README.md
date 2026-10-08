# 浮光

以浮光设计稿和对应的新页面组织创作：项目、画布、镜头、故事板、设定、素材，以及账号与管理功能。正式前端采用当前设计稿的页面实现；功能和数据暂用 mock，真实业务按新需求接入。

## 当前设计入口

- [逐页需求与数据量](workspace/content/requirement/浮光页面/index.md)
- [数据库设计](workspace/content/design/浮光页面数据设计.md)
- [接口设计](workspace/content/design/浮光页面接口设计.md)
- [当前PRD](workspace/content/prd/01-产品需求文档.md)
- [验收标准](workspace/content/requirement/浮光页面非功能与验收.md)

[浮光Figma](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/) 是视觉依据；正式首页为 `/`，代码位于 `frontend/src/components/fuguang/`。旧业务页面、API、数据库结构与工作台不约束当前需求。历史说明隔离在 `workspace/history/`，只供单独安排的迁移或旧环境维护查阅，不进入当前文档导航与搜索。

## 本地阅读

```bash
pnpm --dir workspace install --frozen-lockfile
pnpm --dir workspace exec next dev --webpack --hostname 127.0.0.1 --port 3210
```

默认文档入口 <http://127.0.0.1:3210>。若已有Docker文档服务占用端口，继续使用已有服务，不再启动第二个。生产构建、检索与Obsidian用法见 [知识库说明](workspace/content/KNOWLEDGE.md)。

前端开发服务可在 `frontend` 使用 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`，访问 `/`；启动不代表真实业务接入。旧前端实现已清理；现有数据库与文件数据没有删除或迁移。

协作遵循 [AGENTS](AGENTS.md)，工程工具与门禁见 [PROJECT](PROJECT.md)，当前事项见 [BACKLOG](BACKLOG.md)。第三方许可保持原文，见 [许可](workspace/content/licenses/index.md)。

正式前端替换与浏览器证据见[替换验收](workspace/content/test/浮光正式前端替换验收.md)。

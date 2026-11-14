# 织光 Workspace

同仓的项目知识、Skill 与研发工作流服务。知识原文在 `content/`，页面和索引由同一份原文构建。工程源码、根规范与数据库契约保留原位置。

## 本机使用

在本目录执行：

```bash
pnpm install --frozen-lockfile
pnpm knowledge:sync --repository ..
pnpm build:static
pnpm serve:static
```

静态阅读默认地址为 `http://127.0.0.1:1131/`，部署产物只需静态 HTTP 托管。

在线协作单独构建与启动：

```bash
pnpm build:online
pnpm start:online
```

默认地址为 `http://127.0.0.1:1130/`。访问令牌由启动器保存在 `.local/access-token`，不进入 Git 或构建产物。在线记录需配置受限 PostgreSQL 连接；未配置时仅开放经认证的知识阅读，协作台明确显示不可用。

详细初始化、环境变量、静态与在线打包、音频导入、权限、发布和恢复见[使用与部署指南](content/templates/04-Workspace使用与部署.md)。真实 TTS 生成暂不启用。

## 检查

```bash
pnpm test
pnpm typecheck
pnpm lint
pnpm check:content
pnpm exec playwright test
```

数据库集成测试需显式配置 `GLOWLOOM_WORKSPACE_TEST_DB_DSN`，数据库名称必须以 `glowloom_workspace_test_` 开头。未配置时相应测试显示跳过，不能当作数据库接受。

## 源码与定义

- `src/app/`：共用阅读路由；`src/online/`：在线页面、API 和访问保护。
- `src/server/`：运行记录、提案、执行桥与 Codex 适配；不进入静态产物。
- `skills/`、`workflows/`：唯一 Skill 和流程原文；根发现链接不复制正文。
- `components.json`、`src/components/ui/`、`src/hooks/` 与主题、图标均由 Workspace 独立持有，通过官方 shadcn CLI 维护。
- 数据结构唯一来源为根 `backend/db/schema.sql`；研发运行使用 `dev_workspace`，现有业务 `workspace` schema 保持原归属。

## 界面与 TypeScript 规范

界面遵循根 DESIGN 的 Radix Nova／Neutral／Geist，使用本地官方 shadcn 原语与 Sidebar。Google TypeScript 规范通过 strict、ESLint、Prettier 及边界测试核验。

`src/build/` 负责双模式构建与不可变选版，`src/content/` 负责知识导入／校验，`src/narration/` 负责有权音频导入，`src/runtime/` 负责服务进程，`src/cli.ts` 只作参数与装配。没有 scripts 目录或构建时的业务前端复制。

项目根规范通过 `pnpm knowledge:sync --repository <项目仓库>` 导入 `.local/knowledge/references.json` 受控快照，随独立构建输入一起提供；不复制业务源码或建立第二份可写正文。仅需本目录、锁定依赖及知识输入即可构建，父目录无需 frontend。重新同步后再构建新知识版本。

`pnpm start:online` 仅启动站点。单独运行桥：`pnpm start:bridge --repository <项目仓库>`，站点通过受控 socket／独立令牌连接；本机显式组合使用 `pnpm dev --repository ..`。不连接桥时仍提供认证阅读，执行／落地操作如实显示不可用。

浏览器回归执行 `pnpm exec playwright test`；在线用例要求先启动隔离在线服务，并显式设置 `WORKSPACE_ONLINE_TEST_TOKEN`（可另设 `WORKSPACE_ONLINE_TEST_URL`），不提供内置真实服务令牌。静态用例可单独选择，不连接在线服务。`tests/browser/interface.spec.ts` 补充搜索 URL、键盘焦点、375px／横屏、减少动态效果与双主题 axe 检查。自动扫描与真实产品接受分别记录；朗读测试使用静音 fixture，不调用 TTS。

## 文档底座与部署边界

通用文档能力使用 Fumadocs Core／Headless：普通 Markdown、标题与 TOC、内容源、页面树和相邻导航；界面仍使用本项目 shadcn，不安装另一套完整主题或可执行 MDX。catalog 保留原文档 ID、文件路径和旧链接；Pagefind 是唯一静态搜索索引。

`build:static` 只生成受允许的页面、搜索、raw、图表和既有合法音频，运行时仅需静态托管；内部知识须由托管入口统一保护。`build:online` 单独生成 standalone，按需连接身份、PG、受控执行桥；未配置 PG／桥时仍可认证阅读。静态站不依赖在线 API、数据库、桥或模型。

不建设 TTS 服务、供应商、声音上传或生成调度。既有音频导入／播放只作兼容保留，无资源时完整阅读文字。

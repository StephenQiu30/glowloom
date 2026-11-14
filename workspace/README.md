# 织光 Workspace

同仓的项目知识、Skill 与研发工作流服务。知识原文在 `content/`，页面和索引由同一份原文构建。工程源码、根规范与数据库契约保留原位置。

## 本机使用

在本目录执行：

```bash
pnpm install --frozen-lockfile
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
- 界面原语从现有 `frontend/src/components/ui/` 读取，在忽略的构建目录中派生，不维护第二套手改副本。
- 数据结构唯一来源为根 `backend/db/schema.sql`；研发运行使用 `dev_workspace`，现有业务 `workspace` schema 保持原归属。

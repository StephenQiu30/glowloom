# 管理页统一与五服务接入验证

日期：2026-10-31（Asia/Shanghai）。源码起点 `0a2292d4`，分支main。全部修改保留未提交；本任务未推送、创建PR或发布。

## 设计与页面

- [Figma可编辑统一稿](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3330-65996)：五类桌面管理页、服务接入、连接详情、音乐AK/SK差异、手机、浅色、个人空间和历史主体顶栏；全部文字使用Geist，经字体回读；使用现有组件实例和语义变量，没有整页图片冒充可编辑稿。
- 纠正CR-ADMIN-02后，导航通过既有PageToolbar进入共享ProductHeader。实际桌面1505px测量用户、团队、服务、模型、平台设置：管理导航与账号区域均y=22，且导航在header内。个人空间和历史主体tablist也均在header内。
- 390px与320px，document.scrollWidth分别等于390和320；导航横向滚动，不导致页面横向溢出。手机共享页头为账号首行、导航次行，44px触达。
- 实际页面已确认五入口顺序OpenAI/DeepSeek/火山方舟/豆包语音/火山音乐；OpenAI文本目录出现4个当前模型。未填写或读取真实API Key，未点击真实验证/启用。
- `browser-services.jpg`为实际页面；`figma-services.png`为设计渲染，示例数据不等于实际服务验收。

## OpenAI协议与数据

官方依据：[模型目录](https://developers.openai.com/api/docs/models)、[Responses迁移](https://developers.openai.com/api/docs/guides/migrate-to-responses)、[Images API](https://developers.openai.com/api/reference/resources/images/methods/generate)。当前目录4个文本模型和2个图片模型，不把已退役o1/DALL·E作为新候选。

- 新gpt-6系列文本使用Responses，关闭store与stream，只发送已支持参数；typed output、用量、拒绝、未完成/空响应均有受控回归。
- 历史GPT-4o等冻结版本保留Chat协议及原参数，避免配置更新使历史操作在本地派发前失败。GPT Image及chatgpt-image-latest别名不发送旧response_format，新图片不接受seed/watermark。
- OpenAI文本和单图可以创建release；未验证不能借新发布冒充通过。独立数据库验证了保存、受控验证、显式启用、创作者目录和编辑器报价/确认的冻结release。图片单张产物协议使用内存HTTP传输。
- 旧安装OpenAI增量迁移覆盖missing/existing/custom、重复运行、归档/紧急停用状态保护。
- 实际本地管理页缺少OpenAI预设，使用既有`backend/db/openai_provider_incremental.sql`在当前应用数据库执行事务，确认内置预设1个版本；未重放schema或修改已有连接/凭据。页面随后展示OpenAI入口。
- 浏览器检查期间另有交互创建了OpenAI连接；本任务未填写该连接凭据、未生成或清理该记录。保留用户会话与连接状态。

## 命令与结果

| 检查                                                                                             | 结果                                                                                                           |
| ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| 新增五入口及顶栏回归先失败，再实现                                                               | 已观察到缺失函数/导航仍在正文的失败，修复后通过                                                                |
| OpenAI协议/候选、文本图片发布、冻结编辑器先失败再修复                                            | 已观察到旧Chat路径、退役候选、release不可用、冻结版本缺失导致的失败，修复后通过                                |
| `pnpm exec vitest run`相关providers、page-toolbar、basic-layout、subject-history（maxWorkers=2） | 12文件88项通过                                                                                                 |
| 全量 `pnpm exec vitest run --maxWorkers=2`                                                       | 276文件，1982通过、4失败；不能宣称全量通过                                                                     |
| 修改TSX/TS的ESLint，next typegen，tsc --noEmit，next build                                       | 通过                                                                                                           |
| 修改文件Prettier、gofmt、goimports、git diff --check                                             | 通过                                                                                                           |
| 分离PG18测试库下 `go test -race ./tests/catalog ./tests/operation`                               | 两包通过（23.315s / 79.018s）；需要其他外部服务或专用DSN的测试仍可能跳过，不代表全部外部集成通过               |
| 最后协议调整后的 `go test -race` OpenAI/CloudText/全目录接入专项                                 | 两包通过（6.344s / 3.026s）                                                                                    |
| `go vet ./internal/catalog/... ./internal/operation/...`                                         | 通过                                                                                                           |
| `golangci-lint run ... --new-from-rev=HEAD`                                                      | 0 issues；未使用忽略规则放宽检查                                                                               |
| 未限定增量的catalog/operation golangci-lint                                                      | 初跑24项，含既有其他驱动exported注释、quote_catalog.go格式等；本次触及OpenAI驱动的问题已修正，不扩大到其他文件 |
| `govulncheck ./internal/catalog/... ./internal/operation/...`                                    | 0可达漏洞；1个未被代码调用的依赖模块漏洞提示                                                                   |

全量前端4项失败在未修改的画布范围：business-storage-policy报告canvas-yjs-pending-journal.ts持久存储；advanced-source-combinations的3个组合拒绝提示与断言不一致。相关文件保持HEAD原样，未修改为通过本任务测试。

最初后端全量运行错误地将catalog与operation共用同一隔离库，并使用非预期库名；出现3项专用reference库名断言及1项画布批量确认失败。创建专用glowloom_reference并分离两包后，全量Race通过。未修改相关用例或数据库触发器来规避失败。

## 独立审查

发现并修复3项：图片别名误发response_format、历史预设存在失效新建按钮、历史文本参数收窄导致冻结请求失败。复查确认修复；同步更新历史预设维护断言，维护仍可展开。Portal没有丢失Tabs上下文。

## 边界与运行状态

真实收费API、真实OpenAI模型生成及账号权限验收未执行；所有供应商协议结果使用受控内存传输。旧服务保留管理，新建选择收敛为五项，后台不删除其他适配器与历史记录。

前端沿用已运行的Compose开发服务热更新；后端通过既有Compose服务重启编译本次源码。临时PG18容器与隔离数据在验证后清理；未触及现有数据库的用户、任务、卷或真实凭据。浏览器检查的临时视口已恢复。

## 工作区文件清单

以下逐项列出本任务修改/新增；此前本工作区仅存在本轮调研文件未跟踪。全部保留未提交，未处理用户后续并行变更。

```text
 M BACKLOG.md
 M backend/internal/catalog/adapter/postgres/model_releases.go
 M backend/internal/catalog/domain/adapter_spec.go
 M backend/internal/catalog/domain/model_candidates.go
 M backend/internal/operation/adapter/postgres/editor_submission.go
 M backend/internal/operation/adapter/providerhttp/driver_openai.go
 M backend/tests/catalog/cloud_protocols_test.go
 M backend/tests/catalog/volcengine_onboarding_integration_test.go
 M backend/tests/operation/editor_generation_integration_test.go
 M docs/requirement/04-AI平台接入与预设管理需求.md
 M frontend/src/app/account/_components/personal-space.tsx
 M frontend/src/app/globals.css
 M frontend/src/app/providers/_components/preset-connect.tsx
 M frontend/src/app/providers/_components/provider-label.ts
 M frontend/src/app/providers/_components/providers-page.tsx
 M frontend/src/app/subjects/history/_components/subject-history-page.tsx
 M frontend/src/components/governance/admin-layout.tsx
 M frontend/tests/unit/app/providers/_components/providers-page.test.tsx
 M frontend/tests/unit/components/layout/page-toolbar.test.tsx
?? backend/internal/operation/adapter/providerhttp/openai_responses.go
?? backend/tests/catalog/openai_migration_test.go
?? backend/tests/catalog/openai_protocol_test.go
?? backend/tests/catalog/openai_release_integration_test.go
?? docs/design/2026-10-31-管理页面统一与五服务接入设计.md
?? docs/plan/2026-10-31-管理页面统一与五服务接入计划.md
?? docs/prd/2026-10-31-管理页面统一与五服务接入.md
?? docs/research/2026-10-31-AI服务接入范围与即梦风格调研.md
?? docs/research/evidence/2026-10-31-admin-services/browser-services.jpg
?? docs/research/evidence/2026-10-31-admin-services/figma-services.png
?? docs/research/evidence/2026-10-31-admin-services/validation.md
?? frontend/tests/unit/app/providers/_components/connection-presets.test.ts
```

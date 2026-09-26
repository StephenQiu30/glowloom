# OPS-02 CI/CD 与发布

| 项 | 内容 |
| --- | --- |
| 文档状态 | 草案，待评审（2026-09-25） |
| 上游 | [AGENTS.md](../../AGENTS.md)、[PROJECT.md §9](../../PROJECT.md)、[REQ-02 §9、§13](../requirement/02-非功能需求规格.md)、[TST-01 测试策略](../test/01-测试策略.md)、[TST-03 AI 评测](../test/03-AI评测方案.md)、[OPS-01 环境与部署](01-环境与部署.md) |
| 范围 | 持续集成流水线与质量门禁、制品与版本、部署流程、数据库迁移与工作流版本的发布顺序、回滚、功能开关、依赖更新、发布审批 |

## 1. 原则

1. `main` 始终可构建、可部署；所有合并必须通过 PR 检查。
2. 构建一次、到处部署：同一镜像摘要从 staging 晋升到 production，不重新构建。
3. **生产发布与版本标签须产品负责人明确批准**（AGENTS.md：未经明确要求不发布、不打标签）。
4. 迁移向前兼容一个版本，发布可在 15 分钟内回滚（REQ-02 DEP-03）。
5. 未执行或被跳过的检查在流水线摘要中如实显示，不视为通过。

## 2. 流水线

```text
PR / push 到分支
  ├─ changes：按路径判断受影响的端（backend / agent / frontend / contracts / docs）
  ├─ backend ：gofmt/goimports 检查 → go vet → golangci-lint → go test -race（单元）→ 集成（testcontainers）→ 工作流回放 → govulncheck → wire 生成一致 → swag 生成一致
  ├─ agent   ：ruff check + format --check → mypy → pytest → 契约模型生成一致 → pip-audit → Skill 版本 / hash 校验 →（Skill 变更时）评测门禁
  ├─ frontend：pnpm install --frozen-lockfile → lint → format:check → typecheck → vitest → OpenAPI 客户端生成一致 → pnpm audit → build
  ├─ contracts：JSON Schema 校验 → Go / Python 生成物一致 → 事件 schema 兼容性检查
  ├─ docs    ：链接与锚点校验、需求 / 非功能 / 功能编号校验、TST-02 与功能文件一致
  ├─ security：gitleaks → 依赖许可检查
  └─ e2e-smoke：Compose 启动全部组件 + 模拟供应商 → Playwright 冒烟（TST-01 §6）
合并到 main
  └─ 上述全部 → 构建镜像（backend、agent、frontend）→ Trivy 扫描 → SBOM → 推送镜像仓库（标签 = git sha）→ 自动部署 staging → staging 冒烟
夜间（main）
  └─ 全量 E2E（多浏览器）、性能、故障注入矩阵、fuzz 样本、每周 AI 评测
```

### 2.1 质量门禁汇总

| 门禁 | 阻断 PR | 说明 |
| --- | --- | --- |
| 格式与静态检查（三端） | 是 | PROJECT.md §9 |
| 单元、集成、工作流回放、前端组件测试 | 是 | TST-01 §2 |
| Race Detector | 是 | AGENTS.md |
| 漏洞扫描（高危） | 是 | govulncheck、pnpm audit、pip-audit、Trivy |
| 生成物一致（wire、swag、OpenAPI 客户端、契约模型） | 是 | REQ-02 MNT-03 |
| 迁移可在空库与上一版本库执行 | 是 | DES-02 §10 |
| Secret 扫描 | 是 | DES-07 §13 |
| 文档编号与链接校验 | 是（docs 变更时） | PLN-02 §4.2 |
| AI 评测门禁 | 是（Skill / 模型变更时） | TST-03 §6 |
| E2E 冒烟 | 是 | TST-01 §6 |
| 覆盖率 | 否（核心包 < 80% 时评论提示） | REQ-02 MNT-02 |

CI 运行时长目标：PR 全部检查 ≤ 20 分钟（依赖缓存：Go module、pnpm store、uv cache、Docker layer）。

**当前落地边界（2026-09-27）**：首批工作流直接运行三端格式、静态检查、测试和镜像构建命令，不使用项目脚本或 Makefile。契约生成一致、真实集成、文档校验、安全扫描、端到端冒烟与发布流程仍按 BACKLOG 后续任务实现；尚无远端流水线通过记录，不能将首批工作流视为本节全部门禁通过。

## 3. 制品与版本

| 制品 | 标识 | 存放 |
| --- | --- | --- |
| 容器镜像 | `<仓库>/lanverse/<服务>:<git-sha>`；发布时追加 `:<版本号>` | 火山引擎镜像仓库（CR）【待核实】 |
| SBOM | `sbom-<服务>-<sha>.json` | 随镜像（OCI artifact） |
| 迁移 | `backend/migrations/*.sql`（打包进 backend 镜像） | 镜像内 |
| Skill 包 | 打包进 agent 镜像，`skills/index.json` | 镜像内 |
| 前端 | Next.js standalone 输出打包进镜像 | 镜像内 |

**版本号**：语义化版本 `vMAJOR.MINOR.PATCH`；里程碑发布为 MINOR（如 M1 = v0.1.0，MVP = v1.0.0）；修复为 PATCH。版本标签与 GitHub Release 在产品负责人批准后创建，发布说明列出需求编号、迁移、注意事项。

## 4. 部署流程

### 4.1 staging（自动）

合并到 `main` 后：拉取新镜像 → 执行迁移（`lanverse migrate up`）→ 滚动更新（先 worker、relay，再 api、frontend）→ 冒烟（登录、创建报价、模拟供应商完成一次生成、渲染 2 镜头）→ 结果通知。

### 4.2 production（人工批准）

```text
1. 研发在 GitHub Actions 发起 “deploy-production”，选择已在 staging 验证的镜像 sha
2. 产品负责人在 Environment 审批中批准（GitHub Environments required reviewers）
3. 预检查：staging 冒烟通过；无进行中的 S1 事件；迁移在 staging 已执行；数据库备份 < 24 小时
4. 执行迁移（只允许向前兼容的 expand 类迁移，见 §5）
5. 滚动更新（每个服务先起新实例、健康后停旧实例；backend-api 2 实例保证不中断）
6. 发布后冒烟 + 观察 30 分钟（错误率、延迟、队列积压、unknown 数；OPS-03 看板）
7. 记录发布：版本、sha、迁移、执行人、结果（GitHub Release 或部署记录）
```

发布窗口：工作日 10:00–17:00，避开批量生成高峰；计划停机类变更按 REQ-02 AVL-02 提前 24 小时通知。

## 5. 数据库迁移与发布顺序

采用 **expand / contract** 两阶段：

| 变更 | 发布 N（expand） | 发布 N+1（contract） |
| --- | --- | --- |
| 加列 | 加可空列 / 带默认值；代码兼容新旧 | 回填后加 NOT NULL 约束 |
| 删列 | 代码停止读写该列 | 删除列 |
| 改列名 | 加新列 + 双写 + 回填；读新列 | 删旧列 |
| 加索引 | `CREATE INDEX CONCURRENTLY`（单独迁移） | — |
| 改枚举（CHECK） | 放宽 CHECK（加新值） | 需要时收紧 |

- 迁移在应用滚动更新**之前**执行；任何迁移都必须让“上一版本代码”继续正常工作，保证可回滚。
- 大表回填由后台任务分批执行，不放在迁移事务中。

## 6. 工作流与契约的兼容

| 对象 | 规则 |
| --- | --- |
| Temporal 工作流 | 修改已发布工作流的逻辑必须用 `workflow.GetVersion` 分支；回放测试通过（DES-04 §14）；删除旧分支前确认无在途执行 |
| Activity 契约 | 只做向后兼容变更（加可选字段）；破坏性变更新建 Activity 名（`provider.submit.v2`），旧名保留至在途执行结束 |
| Kafka 事件 | 加字段不升版；语义变化发布 `.v2` 并双写一个发布周期（DES-03 §6.1） |
| 公共 API（`/api`，无 URL 版本） | 只做向后兼容变更；删除字段或改语义先标记废弃并保留一个发布周期；后端先于前端发布；对外开放接口（V2）另设带版本的 `/open/v1` |
| 前端与 API | 前端先兼容新旧响应；部署顺序为后端先、前端后 |
| Worker 部署 | 新旧 Worker 同时存在期间，靠上述版本分支保证兼容；滚动更新后旧 Worker 优雅退出（DES-04 §2） |

## 7. 回滚

| 场景 | 动作 | 目标时长 |
| --- | --- | --- |
| 应用缺陷 | 重新部署上一版本镜像 sha（迁移已保证兼容） | ≤ 15 分钟 |
| 迁移导致问题 | 优先前滚修复；必要时执行对应 down 迁移（仅限 expand 类且无数据丢失） | 视情况 |
| 工作流逻辑缺陷 | 回滚 Worker 镜像；已按新逻辑运行的在途执行通过 `GetVersion` 分支保持一致；必要时对受影响执行发送信号或人工处理 | ≤ 30 分钟 |
| 数据损坏 | 按 OPS-04 恢复流程 | RTO ≤ 4 小时 |

## 8. 功能开关

- 需要分阶段启用的能力（新模型、V2 功能、境外出口）使用简单开关：`workspace.feature_flag`（组织级）或模型注册表的 `status`；开关变更写审计。
- 不引入第三方开关平台；开关在功能稳定后一个版本内移除。

（DES-02 同步项：新增 `workspace.feature_flag`：`key`、`enabled`、`org_id`，含必备列；MVP 可暂不建表，首次需要时通过迁移加入。）

## 9. 依赖更新

- Renovate 每周创建依赖更新 PR（分组：Go、npm、Python、Docker 基础镜像、GitHub Actions）；安全更新即时创建。
- 主要框架（Next.js、Gin、GORM、Temporal SDK、FastAPI）大版本升级作为单独任务评估。
- 基础镜像固定摘要；FFmpeg、libvips 版本升级需运行媒体黄金样例测试（TST-01）。

## 10. CI 中的密钥

- GitHub Actions 只保存：镜像仓库推送凭据、staging / production 部署凭据（Environment secrets，production 需审批）。
- CI 不持有任何模型供应商凭据；需要真实调用的评测由有权限的人手动触发，使用评测专用凭据与预算（TST-03 §7）。

## 11. 待确认

| # | 问题 | 默认处理 |
| --- | --- | --- |
| CD-Q1 | 部署执行方式（SSH + Compose / 部署代理） | 默认 GitHub Actions 通过堡垒机 SSH 执行 `docker compose pull && up -d` |
| CD-Q2 | 镜像仓库 | 默认火山引擎镜像仓库（与生产同地域） |

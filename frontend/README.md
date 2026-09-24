# Lanverse Web 前端

> **当前状态（2026-09-25）：** 本目录下的代码是旧实现（RTK Query、`@umijs/openapi` 生成链、只读画布切片等），不符合新设计，处置方式见 [0401 第 5 节](../docs/design/0401-实施路线与交付计划.md#5-现有代码的处置待确认)。旧实现说明可通过 `git show c99a5528:frontend/README.md` 查看。以下为新设计下本单元的目标职责。

## 职责

| 负责 | 不负责 |
| --- | --- |
| 流水线视图（按阶段推进）、画布视图、审阅、时间线编辑 | 持有供应商密钥，直连 Worker / Temporal / 供应商 |
| 通过后端 REST 读写业务数据，通过 SSE 接收变更通知 | 把本地状态当作已保存的业务事实 |
| 编辑器局部状态（选中、拖拽、视口、撤销栈） | 业务授权与计费判断 |

设计依据：[0101 产品定义](../docs/design/0101-产品定义与需求分析.md)、[0201 §6 画布架构](../docs/design/0201-系统架构设计.md#6-画布架构)、[0301 §5–6](../docs/design/0301-技术选型决策.md)、视觉规范 [DESIGN.md](../DESIGN.md)。

## 技术栈

Next.js App Router、TypeScript strict、pnpm、Tailwind CSS、shadcn/ui（Radix）、TanStack Query、Zustand、React Hook Form + Zod、`@xyflow/react`；测试使用 Vitest、React Testing Library、Playwright。

## 目标目录

```text
frontend/src/
  app/                  # 路由与布局装配
  features/<业务>/      # 业务组件、queries.ts、store.ts（按需）
  features/canvas/      # engine / document / nodes / panels
  components/ui/        # shadcn/ui 基础组件
  lib/                  # api 客户端封装、sse、auth
  gen/api/              # 由 contracts/openapi 生成，禁止手改
```

## 画布要点

1. 引擎使用 React Flow；卡片与交互界面移植自 infinite-canvas（MIT），保留版权声明。
2. 节点只存 `{ref_type, ref_id}`，卡片按 ID 订阅业务数据。
3. 修改表达为命令，松手提交；带 `expected_revision` 与幂等键，409 时基于最新文档重放。
4. 只渲染视口内节点，按缩放级别切换缩略图，视频默认封面、同时播放不超过 3 个。
5. 进入画布开发前须通过 [0301 §6.3](../docs/design/0301-技术选型决策.md#63-方案-c-的落地约束) 的性能 PoC。

完整工程约定见 [PROJECT.md 第 6 节](../PROJECT.md#6-前端)。

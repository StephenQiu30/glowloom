# Agent 服务目录与 TypeScript 规范需求

## Prisma 技术选型确认（SRC-ATS-ORM-02，2026-11-16）

用户原文：

> 我们使用 Prisma

后续方案选择原文：

> Prisma 配类型化查询构建器，保持不手写 SQL（推荐）

确认：普通 CRUD 使用 Prisma，行锁、数据库时钟、条件冲突更新、CTE/LATERAL、窗口预算和微秒游标通过类型化查询构建器补齐；共用同一 Prisma 连接与事务，不新增数据库权威。保持既有协作和恢复合同。该确认覆盖此前 ORM 框架选择，文档目录和规范性 SQL 要求保持 SRC-ATS-ORM-01。

## ORM 数据访问补充（SRC-ATS-ORM-01，2026-11-16）

用户原文：

> 我们的nestjs服务不要手写sql，可以使用orm框架来实现数据的增删改查，减少手写sql造成的不可维护性 同时我们的不需要添加docs这个内容嵌套

确认：NestJS 服务的数据库增删改查、事务、行锁和保存点通过 ORM 完成，不保留直接执行完整 SQL 的旁路；数据库时间、JSON 与窗口统计只使用参数化框架表达式。结构仍以 `backend/db/schema.sql` 为唯一规范性来源，ORM 不自动建表或迁移。Agent 的幂等、owner fencing、未知提交、分页、字节预算及恢复行为保持原合同；Canvas 无数据库职责，不新增数据库连接。文档直接位于 `workspace/content/`，目录专项沿[需求30](30-Workspace官方Fumadocs接入需求.md)维护。

## 1. 来源

记录日期：2026-11-05 至 2026-11-06。

| 来源 ID    | 用户原话                                                                                               | 已确认范围                                                   |
| ---------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------ |
| SRC-ATS-01 | “我们当前的项目中的agent服务需要遵循我们的Google ts的设计规范， 对我们的当前的agent服务进行目录的设计” | 为当前独立 Agent 服务设计目录，并遵循 Google TypeScript 规范 |

### SRC-ATS-02：合同与脚本归入 src

用户后续原话（2026-11-05）：

> 我认为这个设计是不合理的，── contracts/ # HTTP 合同唯一来源
> ├── scripts/ # 生成与维护脚本
> 这两个是不需要的，而且需要整理到我们的src中

已确认：取消包根目录的 `contracts/`、`scripts/`，相关内容整理进 `agent/src/`。当时提出 `src/contracts/`、`src/scripts/`；目录形态以后续 SRC-ATS-04、SRC-ATS-05 为准。

### SRC-ATS-03：Swagger 驱动生成接口文件

用户原话（2026-11-05）：

> 这里我们需要接入 umiopenapi 我们通过swagger接口文档来生成借口文件

已确认：接入 `@umijs/openapi`，通过 Swagger/OpenAPI 文档生成 TypeScript 接口调用及类型文件，不手写生成文件。结合当前服务调用方向，设计落点为 Agent 消费 Go 内部业务接口的客户端；前端已存在的 Umi 生成链继续复用，Go 消费 Agent 的客户端仍使用 Go 生成链。

### SRC-ATS-04：拒绝过度包装与独立脚本

用户原话（2026-11-06）：

> 先检察一下我们的目录设计，我们不需要过度包装和脚本文件

已确认：按实际职责组织源码，取消独立 `scripts/` 目录和常驻的一次性维护、生成脚本。历史维护命令的使用场景在移除前须核对。

### SRC-ATS-05：注解形式生成 Swagger 接口文档

用户原话（2026-11-06）：

> ── contracts/ # 【合并收拢】收回 contracts，作为唯一的 API 契约与 DTO
> │ ├── agent.ts # TypeBox Schemas & TypeScript 类型定义
> │ └── openapi.json # 生成的 OpenAPI 规范文件
> 这里是否有必要，我想要通过注解的形式来生成swagger接口文档，不要手动维护

已确认：取消独立 `contracts/` 目录，基于 Controller 与 DTO 的装饰器生成 Swagger/OpenAPI。规范文件是生成物。文档访问方式由设计37确定。

### SRC-ATS-06：全面切换为 NestJS 框架

用户原话（2026-11-06）：

> 我希望我们的技术框架使用 NestJS来实现 ，切换掉当前的实现

已确认：Agent 服务框架切换为 NestJS。

### SRC-ATS-07（2026-11-06）

> 当前我想要将我们的agent服务的技术的实现切换到基于NestJS来实现，你需要先对我们的设计文件以及需求文件和规范性文件，遵循Google ts 最佳实践来做这部分内容

已确认：先交付需求、设计和工程规范，以 Google TypeScript Style Guide 为编码基线。本轮为文档阶段。

### SRC-ATS-08（2026-11-06）

> 以及不要作兼容性处理，我们的agent服务的完整的技术的选型是什么

已确认：NestJS 单版本切换。交付仅有一套路由、配置、协议和执行入口，移除旧实现、旧合同生成链、版本翻译、双轨和回退。调用方随同次发布更新。

### SRC-ATS-09（2026-11-06）

> 可以，就按这套技术选型和设计开始落地 Agent 服务的 NestJS 迁移，遵循 Google TypeScript 规范，单版本切换，彻底移除旧实现和兼容逻辑，同步更新 Go 调用方并完成测试。

已确认按设计32的技术基线和设计37实施迁移，同步切换 Go 调用方并完成工程验证。

## 2. 解释与边界

| ID | 要求 | 验收条件 |
| --- | --- | --- |
| ATS-01 | Agent 服务采用 NestJS | 由 Nest Module、Controller 与显式 DI 装配一套服务入口。Pi Durable 继续执行和恢复 run，Go 继续负责业务授权、审批、计费和模型出站。 |
| ATS-02 | 源码按职责组织并遵循 Google TypeScript | 单一进程入口，移除独立合同与脚本目录，沿用严格 TypeScript 和 ESM。工程规范明确装饰器、运行时导入与生成物例外。 |
| ATS-03 | 通过注解生成 Swagger，并通过 Umi 生成 TypeScript 接口文件 | Agent 的 OpenAPI 由注册的 Controller/DTO 生成，供 Go 客户端生成。Agent 消费 Go 内部业务接口的 TypeScript 请求与类型从 Go 的独立 Swagger 通过 `@umijs/openapi` 生成。 |
| ATS-04 | 保持产品行为与执行安全 | 按唯一目标合同验证鉴权、错误、SSE、已提交数据和幂等。真实 PostgreSQL 验收覆盖新版本的故障恢复。 |
| ATS-05 | 单版本切换，无兼容处理 | 新版遇到不支持的旧输入或存量执行定义时明确拒绝。切换前收束无法直接接续的在途运行，同次更新调用方并移除旧实现。 |

`FastifyAdapter`、DTO 校验方式、文档端点及目录细分是[设计37](../design/37-Agent服务目录与TypeScript规范设计.md)的工程方案。[设计32 §7.1.2](../design/32-完整创作AgentHarness与Skill多Agent架构设计.md#712-已确定的技术栈与版本基线)集中维护目标技术栈与版本。

## 3. 来源追踪

SRC-ATS-10（2026-11-12）：本轮完整原文见[需求19 SRC-AGH-22](19-完整创作Agent与Skill多Agent需求.md#src-agh-22)。重新评估当前技术栈、已实施目录和双向生成链的合理性及可用证据；沿已确认ATS-01～05，不把“评估”解释为另换框架、重做迁移或新增根级contracts/scripts/services。新增工程决策归设计37，实施仍沿PRD19/Plan28；仅文档。

目录专项见[设计37](../design/37-Agent服务目录与TypeScript规范设计.md)。执行权威与服务边界继续由[设计32](../design/32-完整创作AgentHarness与Skill多Agent架构设计.md)维护，对话合同继续由[设计35](../design/35-对话工作台与参考输入一致性设计.md)维护；工程规则见 [agent/AGENTS.md](../../../agent/AGENTS.md)。本专项不复制产品 PRD 或另造产品功能。

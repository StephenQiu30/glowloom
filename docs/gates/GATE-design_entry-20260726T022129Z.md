---
gate_scope: design_entry
result: passed
input_versions: [DESIGN-01/1.2.0@4a3af3c193b6c9d3210ff1352e46a989fb87e052, DESIGN-02/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-03/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-04/1.2.0@a6c407eb9d7c720d2a1bcf49ac1b7a5f5e965e3d, DESIGN-05/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-06/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-07/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-08/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-09/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-10/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-11/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-12/1.0.0@a6c407eb9d7c720d2a1bcf49ac1b7a5f5e965e3d, DESIGN-13/1.2.0@a6c407eb9d7c720d2a1bcf49ac1b7a5f5e965e3d]
owner: Lanverse Architecture
reviewers: [Product Owner (current task approval), Codex (Frontend architecture review)]
decided_at: 2026-07-26T02:21:29Z
supersedes: GATE-design_entry-20260726T010001Z
gaps: []
evidence: [gate:GATE-requirement_readiness-20260726T022128Z, git:a6c407e, reference:umijs-openapi-native-api-directory]
next_stage: prd_and_plan_review
---

# Design entry 前端生成目录门禁

## 1. 结论

十三份 Design 保持 `accepted`。DESIGN-04、12、13 一致规定 `serversPath` 指向 `src/`，客户端整体发布到生成器原生 `src/api/`，结论为 `passed`。

## 2. 一致性检查

| 检查项 | 结果 | 说明 |
| --- | --- | --- |
| 生成边界 | passed | `src/api/` 只含 Swagger 生成物，可整体替换和清理过期 Operation |
| 手写边界 | passed | RTK Query 缓存/轮询位于 `store/backend-api.ts`，不手写请求接口 |
| 依赖方向 | passed | 页面不得直接调用生成函数，唯一 sender 仍为 `lib/request.ts` |
| 回滚 | passed | 仅移动可再生客户端文件，不触碰数据库、对象存储或业务事实 |

## 3. 失效条件

DESIGN-04、12、13 的生成路径、调用边界或唯一契约源变化时，本记录失效。

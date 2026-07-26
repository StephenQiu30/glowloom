---
gate_scope: design_entry
result: passed
input_versions: [DESIGN-01/1.2.0@4a3af3c193b6c9d3210ff1352e46a989fb87e052, DESIGN-02/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-03/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-04/1.2.0@4a3af3c193b6c9d3210ff1352e46a989fb87e052, DESIGN-05/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-06/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-07/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-08/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-09/1.1.0@729f9c5504b8b8338cfa410edb85a46d1d153331, DESIGN-10/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-11/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-12/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, DESIGN-13/1.2.0@4a3af3c193b6c9d3210ff1352e46a989fb87e052]
owner: Lanverse Architecture
reviewers: [Product Owner (current task approval), Codex (FastAPI and packaging review)]
decided_at: 2026-07-26T01:00:01Z
supersedes: GATE-design_entry-20260725T160001Z
gaps: []
evidence: [gate:GATE-requirement_readiness-20260726T010000Z, git:4a3af3c, reference:setuptools-src-layout]
next_stage: prd_and_plan_review
---

# Design entry 直接源码布局门禁

## 1. 结论

十三份 Design 保持 `accepted`。DESIGN-04 v1.2.0 将唯一物理结构固定为 `backend/src/{api,core,db,domain,integrations,repositories,resources,schemas,services,workers}` 与两个顶层入口；设计允许继续进入 Plan，结论为 `passed`。

## 2. 一致性检查

| 检查项 | 结果 | 说明 |
| --- | --- | --- |
| 单一分层 | passed | 物理技术层直接位于 src，逻辑业务域不映射为重复 package |
| 依赖方向 | passed | 去掉包装层不改变 API、Service、Repository、Domain、Worker 方向 |
| 可安装性 | passed | Setuptools 显式发现 package，并分发 main/worker 顶层模块 |
| 回滚 | passed | 目录与导入重构不执行数据库或对象存储变更 |

## 3. 失效条件

DESIGN-04 的目录、构建后端、入口或依赖方向变化时，本记录失效。

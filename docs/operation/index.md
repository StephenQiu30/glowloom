---
id: OPS-CURRENT
title: "当前运行边界"
type: operation
status: unreviewed
tags: [fuguang, current-design]
---

# 当前运行边界

当前阶段不部署新业务，也不根据旧运维方案提前建设中间件。新模型/接口接受后，运行方案再按实际异步任务、私有对象、凭据、备份恢复及可观测需求设计。

仓库中的旧服务和用户数据保持原状。旧部署与故障记录存于 `docs/history/operation/`，只有处理明确的旧环境运维/迁移任务时才读取；不作为新产品架构的默认要求。

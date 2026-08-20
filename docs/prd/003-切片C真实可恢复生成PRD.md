# 切片 C：真实可恢复生成 PRD

> PRD ID：PRD-C
> 上游：切片 A；可不依赖切片 B；M07、M08、M09、M11、M14
> 状态：proposed

## 1. 用户问题与结果

用户需要在看见输入、Prompt、能力、资源和外发风险后，批准真实图像/视频生成；即使 Provider 超时、重复回调或下载中断，也不会重复扣费或丢失候选。

## 2. P0 旅程

选择 Shot → 创建/检查 GenerationPlan → 查看编译 Prompt、参考、能力和淘汰原因 → 查看资源区间/硬上限和治理结论 → 批准 start_now 或 hold → 观察每项 Job/Attempt → unknown 对账 → 媒体摄取 → 候选比较和人工 Selection。

## 3. 首期能力范围

- 一个真实图像 capability、一个真实视频 capability；
- 同步/轮询/回调按实际 Provider 选择，不要求为演示同时实现三种；
- Provider 必须在进入实现前签认幂等、查询、取消、回调、保留、训练和计量证据；
- 此切片不宣称同类视频多模型路由完成，两个同类 capability 另设 Gate。

## 4. 页面

生成计划、外发披露/治理阻塞、任务与异常中心、unknown 对账、候选比较、项目用量。所有批量状态可下钻单 PlanItem/Job。

## 5. 故障场景

提交响应前超时、重复/乱序回调、Worker 在关键点重启、Provider 限流、部分失败、取消未知、下载中断/哈希不一致、Provider 账户撤销、用量迟到。每个场景必须有用户可见状态和安全下一动作。

## 6. 验收与退出

1. approved Plan 输入不可被后续 Shot/实体修改改变；
2. hold 不启动，start_now/重复命令只产生一个 Job；
3. 提交 unknown 不盲重试，可显示对账证据；
4. 重复回调不产生重复 Candidate/UsageEntry；
5. 下载失败复用 external ID，不重新生成；
6. 每个 Candidate 可还原 Plan、Prompt、Attempt、模型、输入、权利和用量；
7. 人工主选不被新 Candidate/评分覆盖；
8. 超过硬上限或治理不通过时 Provider 调用次数为零。

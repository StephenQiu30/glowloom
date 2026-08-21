# Lanverse Acceptance 状态

- 状态：waiting_for_implementation
- 最近审查：2026-08-22

当前没有目标方案的真实验收证据。Requirement 中的 `AC-SYS-*` 以及 M01—M15 的 `AC-IAM-*`、`AC-PRJ-*`、`AC-NAR-*`、`AC-KNO-*`、`AC-SHT-*`、`AC-AIC-*`、`AC-PLN-*`、`AC-EXE-*`、`AC-MED-*`、`AC-QAR-*`、`AC-USG-*`、`AC-REV-*`、`AC-DLV-*`、`AC-GOV-*` 和 `AC-INT-*` 都是实施前验收条件，不代表已经通过。

实现完成后，每份证据必须关联需求 ID、固定测试数据、环境、执行命令、结果、失败边界和残余风险。剧本分析首步使用同一份不少于 3 集的金标整本脚本，覆盖明确/缺失/冲突集标题、跨集别名、同名反证、M04 UnresolvedSubjectRevision 未决人物行及清空投影后的成员/行键一致性、删除一个 current MentionResolution、制造 active 未决成员重叠、Occurrence failed scopes/unassigned/overlap、地点复用、道具、服装、遗漏一个 coverage subject 时 `unassessed>0`、零 Requirement 引用的 not_required/rejected row、ProductionCoverageSchemaRevision 换版 stale 与按旧 hash 重建、参考/媒体变化只重建 readiness，以及单集部分失败，并验证 `AC-PRJ-009`、`AC-NAR-010—012`、`AC-KNO-013—016`。后续镜头主线再执行换装、地点状态和镜头重排的中途变更实验；文档、Mock、代码存在或测试源码不能代替真实执行结果。

每份未来 Evidence 在文件内直接链接对应 Requirement ID、Design、PRD 和 Plan，不再建立独立追踪目录。被替代的验收文件从当前事实源移除；历史 Git 记录不证明当前需求已经通过。

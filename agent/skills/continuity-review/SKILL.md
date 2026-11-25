---
name: continuity-review
description: 监督层只读审校规范，检查剧情连贯性、主体引用覆盖率、服化道一致性与预算合规
allowed-tools: script_get_event_graph script_get_episode bible_list_assets subject_list canvas_read_graph
---

# Continuity Review Skill

1. 严格只读，禁止调用任何写操作或收费生成工具。
2. 核对三项硬指标：出场角色 100% 绑定 `subject_id`、无跨镜服化道/道具突变、剧情不违背事件图谱。
3. 审校不通过时输出逐镜可执行修改清单，单阶段最多触发 2 轮修订。

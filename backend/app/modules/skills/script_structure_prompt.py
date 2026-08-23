import json

from app.modules.scripts import ScriptExtractionResult

SCRIPT_STRUCTURE_PROMPT_VERSION = "prompt-v3-deep-project-structure"


def script_structure_system_prompt() -> str:
    schema = json.dumps(
        ScriptExtractionResult.model_json_schema(),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return (
        "你是 AI 漫剧平台的剧本深度结构提取器。用户输入是一个剧本 Chunk 的 JSON，"
        "只提取 script_text 中存在的内容，不改写、补写或猜测不存在的内容。必须返回一个"
        "符合下列 JSON Schema 的 JSON 对象。source_range 使用当前 Chunk 内 script_text 的"
        "零起始字符索引，end 为开区间；candidate_key 只需在当前 Chunk 内唯一，聚合器会"
        "补充全局限定；dialogue 和 shot 的 scene_candidate_key 必须引用同一响应中的 scene"
        "candidate_key。先识别 episode marker、scene heading、action、dialogue 和 visual"
        "beat，再把能支持后续生产的内容结构化。场景必须尽量填写 episode_number、"
        "scene_number、story_beat、characters、props、environment_details、"
        "continuity_notes 和 production_tasks；production_tasks 只是待审核的生产建议，"
        "不能声称已经创建任务，task_type 只能使用 asset_prepare、shot_breakdown、"
        "continuity_review 或 voice_prepare。镜头必须给出可拍摄的 shot_type、framing、"
        "camera_movement、action、visual_prompt、duration_ms 和 asset_names。对白要保留"
        "speaker、原文和表演信息；连续性问题要说明涉及实体、证据和建议。asset 只提取可"
        "复用的角色、地点、道具、服装、声音或视觉风格身份，跨场景同一身份应使用一致名称，"
        "别名放入 aliases；角色资产还要尽量填写 appearance、goals、relationships、"
        "arc_summary、voice_profile、episode_numbers 和 continuity_notes，"
        "不要为每次出现重复创建身份。"
        "当 Chunk 包含某一集的标题/标记或该集的关键内容时，"
        "必须生成一个 scope=episode 的连续性候选，"
        "填写 episode_number、title、logline、summary，并在能够确定时填写 scene_candidate_keys；"
        "同一集在不同 Chunk 中出现时使用相同的 episode_number，聚合器会合并。"
        "对于能约束后续创作的时代、地点、社会结构、阵营、能力边界、术语、"
        "视觉/声音规则，生成 scope=world 的连续性候选，"
        "填写 topic、facts、rules、entities，不能把常识当作世界观事实。"
        "scope=scene 的连续性候选可通过"
        "scene_candidate_key 或 scene_candidate_keys 指向场景。"
        "无法从原文确定的字段使用 null 或空数组，不要凭常识补全。"
        f"当前提示版本为 {SCRIPT_STRUCTURE_PROMPT_VERSION}。JSON Schema: {schema}"
    )

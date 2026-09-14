// Browser-only fixtures. Intercepts API traffic; never writes to the backend.
// node tests/browser/creation-preview.mjs [install|approved|adopted|readonly]
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const session = "lanverse-creation-fixture";
const mode = process.argv[2] ?? "install";
if (!["install", "approved", "adopted", "readonly"].includes(mode)) throw new Error("Unknown preview state");
const projectId = "51000000-0000-4000-8000-000000000001";
const sourceId = "51000000-0000-4000-8000-000000000002";
const runId = "51000000-0000-4000-8000-000000000003";
const episodeId = "51000000-0000-4000-8000-000000000004";
const hash = "a".repeat(64);
const created = "2026-09-15T08:00:00Z";
const evidence = { block: 0, quote: "阿宁披着红衣，手臂受伤。", occurrence: null };
const episodeMap = { mode: "preserve", episodes: [{ key: "ep", number: 1, title: "雨夜来信", first_block: 0, last_block: 2, rationale: "保留原稿集划分。来信引出人物冲突。" }], excluded: [], issues: [] };
const analysis = { episode_key: "ep", summary: "阿宁带伤归来，门外有人送来一封信。", conflict: "是否打开来信", turning_point: "门外传来敲门声", ending_hook: "送信人没有回答", excluded: [], issues: [], scenes: [{ key: "door", title: "旧宅门前", first_block: 0, last_block: 2, summary: "阿宁在旧宅门前等待，听到敲门声。", time_label: "夜", time_branch: "主时间线", presentation: "present", beats: [{ key: "wait", action: "阿宁的手停在门把上", evidence: [evidence], required: true, origin: "extracted" }], dialogues: [{ key: "question", speaker_mention: "person", text: "这么晚了，是谁？", evidence, channel: "onscreen" }], mentions: [{ key: "person", name: "阿宁", kind: "cast", presence: "onscreen", evidence, visual_details: [evidence] }, { key: "house", name: "旧宅", kind: "place", presence: "onscreen", evidence, visual_details: [] }], issues: [{ code: "time", scope: "ep/door", severity: "warning", summary: "送信人的身份尚未揭示，不应自行补全。" }] }] };
const world = { entities: [{ key: "aning", label: "阿宁", kind: "cast", identity_basis: "explicit", uncertainty: null, mentions: [{ episode_key: "ep", scene_key: "door", mention_key: "person" }], evidence: [evidence] }, { key: "house", label: "旧宅", kind: "place", identity_basis: "explicit", uncertainty: null, mentions: [{ episode_key: "ep", scene_key: "door", mention_key: "house" }], evidence: [evidence] }], relations: [], state_events: [
  { entity_key: "aning", episode_key: "ep", scene_key: "door", time_branch: "主时间线", story_time: "归来时", property: "手臂", before: "完好", after: "受伤", knowledge: "known", basis: "narration", evidence: [evidence] },
  { entity_key: "aning", episode_key: "ep", scene_key: "door", time_branch: "主时间线", story_time: "包扎后", property: "手臂", before: "受伤", after: "已包扎", knowledge: "known", basis: "narration", evidence: [evidence] },
], asset_needs: [{ entity_key: "aning", description: "红衣与伤后包扎形象，保持同一角色身份。", evidence: [evidence] }], unresolved_mentions: [], issues: [] };
const direction = { episode_key: "ep", scene_key: "door", dramatic_intent: "让观众感受到开门前的犹豫", audience_knows: ["阿宁刚刚受伤归来"], withhold: ["送信人的身份"], blocking: [{ mention_key: "person", position: "门内右侧", facing: "房门", action: "手伸向门把" }], issues: [], shots: [
  { key: "hand", purpose: "呈现迟疑", framing: "手部特写", camera_movement: "固定", action: "手指停在门把上", beat_keys: ["wait"], audio: [{ dialogue_key: "question", channel: "onscreen" }], visible_mentions: ["person"], detail_evidence: [evidence], duration_min_ms: 1000, duration_max_ms: 3000, timing_basis: "停顿和对白", screen_direction: "从右向左", entry_state: "手伸向门", exit_state: "手停在门把上", panel_caption: "包扎的手停在门把上。" },
  { key: "door", purpose: "保留未知", framing: "门口中景", camera_movement: "缓慢推进", action: "阿宁望向门外", beat_keys: ["wait"], audio: [], visible_mentions: ["person", "house"], detail_evidence: [evidence], duration_min_ms: 2000, duration_max_ms: 4000, timing_basis: "等待回应", screen_direction: "正面", entry_state: "门仍关闭", exit_state: "未揭示来客", panel_caption: "旧宅房门紧闭，敲门声停下。" },
] };
const stages = ["map_manuscript", "analyze_episode", "build_world", "direct_scene"];
const candidates = [episodeMap, analysis, world, direction];
const receipt = { submission_id: "fixture-adoption", formal_refs: [{ type: "text_intent_version", id: "fixture-text-intent", revision: 1 }], risk_resolutions: [{ code: "continuity_mapping_pending", scope: "ep/door", reason: "已核对，保留伤后状态并隐藏来客身份。" }] };
const proposals = stages.map((stage, index) => ({ id: `fixture-proposal-${index}`, run_id: runId, step_id: `fixture-step-${index}`, stage, step_key: stage, revision: 1, result_hash: hash, candidate_hash: hash, source_revision_id: sourceId, source_hash: hash, invocation_id: `fixture-invocation-${index}`, input_hash: hash, release_hash: hash, human_task_id: `fixture-task-${index}`, candidate: candidates[index], evidence: [{ revision_id: sourceId, source_hash: hash, block: 0, start: 0, end: 15, text_hash: hash, quote: evidence.quote }], issues: index === 3 ? [{ code: "continuity_mapping_pending", scope: "ep/door", severity: "blocker", summary: "核对人物伤后状态与来客身份披露" }] : [], status: index < 3 || mode === "adopted" ? "accepted" : "needs_review", acceptance: index < 3 || mode === "adopted" ? receipt : null, created_at: created }));
const run = { id: runId, project_id: projectId, workspace_id: projectId, source: { document_id: sourceId, revision_id: sourceId, revision: 1, content_hash: hash, span_index_id: sourceId }, flow_type: "lanverse.creation.text-storyboard.production", workflow_id: `fixture:${runId}`, status: "accepted", revision: 2, last_error: "", acceptance: null, created_at: created, updated_at: created };
const me = { user: { id: "fixture-user", display_name: "流程验证账号", email: "fixture@example.invalid", avatar_url: null }, workspace: { id: projectId, name: "隔离测试 · 非真实作品", role: mode === "readonly" ? "viewer" : "owner", status: "active", revision: 1 } };
const project = { id: projectId, workspace_id: projectId, name: "雨夜来信 · 流程测试", description: "仅用于前端流程验证，未运行模型、未写入数据库。", aspect_ratio: "9:16", language: "zh-CN", visual_style: "水墨", target_duration_ms: 90000, status: "active", revision: 1 };
function browser(...args) { return execFileSync("agent-browser", ["--session", session, ...args], { encoding: "utf8", env: { ...process.env, AGENT_BROWSER_INIT_SCRIPTS: fileURLToPath(new URL("./creation-preview-init.js", import.meta.url)) } }); }
const routes = [];
function route(path, data) {
  const url = `http://127.0.0.1:8123/__browser_fixture${path}`;
  routes.push({ url, body: JSON.stringify({ data }).replace(/[^\x00-\x7F]/g, (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`) });
}

if (mode === "install" || mode === "readonly") browser("open", "about:blank");
browser("network", "unroute", "http://127.0.0.1:8123/__browser_fixture/**");

{
  route("/api/auth/refresh", { ...me, access_token: "isolated-browser-fixture-not-a-real-token", token_type: "bearer", expires_in: 1800 });
  route("/api/me", me);
  route(`/api/projects/${projectId}`, project);
  route(`/api/projects/${projectId}/current-script-document`, { document: { title: "雨夜来信.md" }, revision: { id: sourceId, normalized_hash: hash, normalized_text: `第一集 雨夜来信\n${evidence.quote}\n阿宁：这么晚了，是谁？` } });
  route(`/api/projects/${projectId}/current-script-source`, { identity: { version_id: sourceId, content_hash: hash } });
  route(`/api/projects/${projectId}/creation-runs*`, [run]);
  route(`/api/creation-runs/${runId}`, run);
  route(`/api/creation-runs/${runId}/sync-proposals`, { synced: true });
  const episode = { id: episodeId, project_id: projectId, name: "雨夜来信", position: 1, target_duration_ms: 90000, current_script_version_id: sourceId };
  route(`/api/projects/${projectId}/episodes`, [episode]);
  route(`/api/episodes/${episodeId}`, episode);
  route(`/api/episodes/${episodeId}/structure`, { id: "fixture-structure", status: "confirmed", revision: 1, script_version_id: sourceId, scenes: [{ id: "fixture-door", heading: "旧宅门前", position: 1, narrative_units: [{ id: "fixture-action", kind: "action", text: evidence.quote }], dialogues: [{ id: "fixture-dialogue", speaker: "阿宁", text: "这么晚了，是谁？" }], tasks: [{ id: "fixture-breakdown", kind: "shot_breakdown", label: "拆解场景分镜", status: "accepted", required: true }] }] });
  route(`/api/episodes/${episodeId}/storyboard-draft`, null);
  route(`/api/episodes/${episodeId}/storyboard-export`, null);
  route(`/api/episodes/${episodeId}/shots`, []);
}
route(`/api/creation-runs/${runId}/proposals`, proposals);
route(`/api/creation-runs/${runId}/execution`, { status: mode === "adopted" ? "completed" : "waiting_review", stage: "direct_scene", call_limit: 20, reserved_calls: 4, last_error: "", can_resume: false, steps: stages.map((stage) => ({ step_key: stage, state: "needs_review" })) });
for (let index = 0; index < stages.length; index++) {
  const task = { id: `fixture-task-${index}`, status: "CLAIMED", revision: 2, claim: { claim_token: "fixture-claim" }, subject_revision: 1, subject_hash: hash };
  const decision = { id: "fixture-decision", decision: "approved" };
  route(`/api/human-tasks/fixture-task-${index}`, { task, decision: mode === "approved" || mode === "adopted" ? decision : null, coordination: null });
  route(`/api/human-tasks/fixture-task-${index}/decisions`, { task, decision, coordination: null });
  route(`/api/creation-runs/${runId}/proposals/fixture-proposal-${index}/adopt`, receipt);
}
// Matching is prefix-based: register specific endpoints before their parents.
for (const { url } of routes) browser("network", "unroute", url);
for (const { url, body } of routes.sort((left, right) => right.url.length - left.url.length)) {
  browser("network", "route", url, "--body", body);
}
// Register the catch-all last: agent-browser uses the first matching route.
browser("network", "route", "http://127.0.0.1:8123/__browser_fixture/**", "--body", JSON.stringify({ error: { code: "fixture_unconfigured", message: "未配置的测试接口，不访问真实后端" } }));
if (mode === "install" || mode === "readonly") {
  console.log(browser("open", `http://127.0.0.1:8123/projects/${projectId}/creation`));
}
console.log(`Fixture session: ${session}; state: ${mode}; no database or model calls.`);

import type { ReactNode } from "react";

export const stageLabels: Record<API.CreationProposal["stage"], string> = {
  map_manuscript: "分集方案", analyze_episode: "剧稿结构", build_world: "世界设定", direct_scene: "导演分镜",
};
const channelLabels: Record<string, string> = { onscreen: "画内", offscreen: "画外", phone: "电话", inner: "内心", group: "群声", unknown: "待确认" };
const kindLabels: Record<string, string> = { cast: "角色", place: "场景", prop: "道具" };
const originLabels: Record<string, string> = { extracted: "原文提取", inferred: "推断", proposed: "创作建议" };
const presentationLabels: Record<string, string> = { present: "当前叙事", flashback: "闪回", dream: "梦境", intercut: "交叉叙事", montage: "蒙太奇", unknown: "待确认" };
const basisLabels: Record<string, string> = { narration: "叙述事实", claim: "人物声称", unknown: "依据待确认" };
const knowledgeLabels: Record<string, string> = { known: "已知", unknown: "未知", conflicting: "存在矛盾" };
const excludedLabels: Record<string, string> = { heading: "标题", author_note: "作者说明", non_story: "非剧情", unresolved: "待确认" };
function Facts({ children }: { children: ReactNode }) { return <dl className="grid gap-3 text-sm sm:grid-cols-2">{children}</dl>; }
function Fact({ label, children }: { label: string; children: ReactNode }) { return <div><dt className="text-muted-foreground">{label}</dt><dd className="mt-1 whitespace-pre-wrap break-words">{children || "无"}</dd></div>; }
function Quotes({ items }: { items: API.CreationTextEvidence[] }) { return <ul className="mt-2 space-y-1 border-l-2 pl-3 text-xs text-muted-foreground">{items.map((item, i) => <li key={i}>原文块 {item.block + 1}：「{item.quote}」{item.occurrence != null ? `（第 ${item.occurrence + 1} 处）` : ""}</li>)}</ul>; }
function Range({ value }: { value: { first_block: number; last_block: number } }) { return <span>原文块 {value.first_block + 1}–{value.last_block + 1}</span>; }

export function ProposalContent({ proposal, proposals }: { proposal: API.CreationProposal; proposals: API.CreationProposal[] }) {
  if (proposal.stage === "map_manuscript") {
    const value = proposal.candidate as API.CreationTextEpisodeMap;
    return <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{value.mode === "preserve" ? "保留原稿集划分" : "建议重新划分剧集"}</p>
      {value.episodes.map((episode, i) => <section className="space-y-2 rounded-md border p-4" key={episode.key}><h3 className="font-semibold">第 {episode.number ?? i + 1} 集 · {episode.title}</h3><p className="text-xs text-muted-foreground"><Range value={episode} /></p><p className="whitespace-pre-wrap text-sm">{episode.rationale}</p></section>)}
      {value.excluded.length > 0 && <section><h3 className="font-semibold">未纳入剧集的原文</h3><ul className="mt-2 space-y-2 text-sm">{value.excluded.map((item, i) => <li key={i}><Range value={item} /> · {excludedLabels[item.kind]}：{item.reason}</li>)}</ul></section>}
    </div>;
  }
  if (proposal.stage === "analyze_episode") {
    const value = proposal.candidate as API.CreationTextEpisodeAnalysis;
    return <div className="space-y-5"><Facts><Fact label="剧情概述">{value.summary}</Fact><Fact label="主要冲突">{value.conflict}</Fact><Fact label="转折">{value.turning_point}</Fact><Fact label="结尾悬念">{value.ending_hook}</Fact></Facts>
      {value.scenes.map((scene, i) => <section className="space-y-4 rounded-md border p-4" key={scene.key}>
        <h3 className="font-semibold">第 {i + 1} 场 · {scene.title}</h3><p className="text-xs text-muted-foreground"><Range value={scene} /> · {scene.time_label} · {scene.time_branch === "unknown" ? "时间线待确认" : scene.time_branch} · {presentationLabels[scene.presentation]}</p><p className="whitespace-pre-wrap text-sm">{scene.summary}</p>
        <div><h4 className="text-sm font-medium">剧情节拍</h4><ol className="mt-2 space-y-3">{scene.beats.map((beat) => <li className="text-sm" key={beat.key}>{beat.required ? "必需 · " : ""}{beat.action} <span className="text-xs text-muted-foreground">（{originLabels[beat.origin]}）</span><Quotes items={beat.evidence} /></li>)}</ol></div>
        <div><h4 className="text-sm font-medium">对白与声音</h4><ul className="mt-2 space-y-3">{scene.dialogues.map((dialogue) => <li className="text-sm" key={dialogue.key}><span className="text-muted-foreground">{scene.mentions.find((item) => item.key === dialogue.speaker_mention)?.name ?? "未指明说话人"} · {channelLabels[dialogue.channel]}：</span>{dialogue.text}<Quotes items={[dialogue.evidence]} /></li>)}</ul></div>
        <div><h4 className="text-sm font-medium">人物、场景与道具</h4><ul className="mt-2 space-y-3">{scene.mentions.map((mention) => <li className="text-sm" key={mention.key}>{kindLabels[mention.kind]} · {mention.name} · {channelLabels[mention.presence] ?? (mention.presence === "mentioned" ? "仅被提及" : "待确认")}<Quotes items={[mention.evidence, ...mention.visual_details]} /></li>)}</ul></div>
      </section>)}
      {value.excluded.map((item, i) => <p className="text-sm" key={i}><Range value={item} /> · 未纳入场景：{item.reason}</p>)}
    </div>;
  }
  if (proposal.stage === "build_world") {
    const value = proposal.candidate as API.CreationTextWorldBook;
    return <div className="space-y-5">
      {value.entities.map((entity) => <section className="space-y-2 rounded-md border p-4" key={entity.key}><h3 className="font-semibold">{entity.label} <span className="text-sm font-normal text-muted-foreground">{kindLabels[entity.kind]}</span></h3><p className="text-sm">身份依据：{entity.identity_basis === "explicit" ? "原文明确" : entity.identity_basis === "inferred" ? "推断" : "待确认"}</p>{entity.uncertainty && <p className="text-sm">不确定项：{entity.uncertainty}</p>}<p className="text-xs text-muted-foreground">关联提及：{entity.mentions.map((item) => `${item.episode_key}/${item.scene_key}/${item.mention_key}`).join("、")}</p><Quotes items={entity.evidence} /></section>)}
      <section className="space-y-3"><h3 className="font-semibold">关系与状态</h3>{value.relations.map((item, i) => <div className="text-sm" key={`relation:${i}`}>{value.entities.find((e) => e.key === item.subject)?.label} → {item.predicate} → {value.entities.find((e) => e.key === item.target)?.label}（{originLabels[item.origin]} / {basisLabels[item.basis]}）<Quotes items={item.evidence} /></div>)}{value.state_events.map((item, i) => <div className="text-sm" key={`state:${i}`}>{value.entities.find((e) => e.key === item.entity_key)?.label} · {item.property}：{item.before ?? "未知"} → {item.after ?? "未知"}<p className="text-xs text-muted-foreground">{item.episode_key}/{item.scene_key} · {item.time_branch} · {item.story_time} · {knowledgeLabels[item.knowledge]} / {basisLabels[item.basis]}</p><Quotes items={item.evidence} /></div>)}</section>
      {value.unresolved_mentions.length > 0 && <p className="text-sm">未明确归属的提及：{value.unresolved_mentions.map((item) => `${item.episode_key}/${item.scene_key}/${item.mention_key}`).join("、")}</p>}
      {value.asset_needs.length > 0 && <section><h3 className="font-semibold">后续视觉参考需求</h3>{value.asset_needs.map((item, i) => <div className="mt-3 text-sm" key={i}>{value.entities.find((e) => e.key === item.entity_key)?.label}：{item.description}<Quotes items={item.evidence} /></div>)}</section>}
    </div>;
  }
  const value = proposal.candidate as API.CreationTextSceneDirection;
  const analysis = proposals.find((item) => item.stage === "analyze_episode" && (item.candidate as API.CreationTextEpisodeAnalysis).episode_key === value.episode_key)?.candidate as API.CreationTextEpisodeAnalysis | undefined;
  const scene = analysis?.scenes.find((item) => item.key === value.scene_key);
  return <div className="space-y-5"><Facts><Fact label="戏剧意图">{value.dramatic_intent}</Fact><Fact label="观众已知">{value.audience_knows.join("；")}</Fact><Fact label="暂不揭示">{value.withhold.join("；")}</Fact></Facts>
    {value.blocking.length > 0 && <section><h3 className="font-semibold">人物调度</h3>{value.blocking.map((item, i) => <p className="mt-2 text-sm" key={i}>{scene?.mentions.find((m) => m.key === item.mention_key)?.name ?? item.mention_key}：{item.position}，面向 {item.facing}，{item.action}</p>)}</section>}
    {value.shots.map((shot, i) => <section aria-label={`镜头 ${i + 1}`} className="space-y-4 rounded-md border p-4" key={shot.key}><h3 className="font-semibold">镜头 {i + 1} · {shot.framing}</h3><p className="whitespace-pre-wrap text-sm">{shot.panel_caption}</p><Facts><Fact label="镜头目的">{shot.purpose}</Fact><Fact label="画面动作">{shot.action}</Fact><Fact label="运镜">{shot.camera_movement}</Fact><Fact label="屏幕方向">{shot.screen_direction}</Fact><Fact label="时长范围">{shot.duration_min_ms / 1000}–{shot.duration_max_ms / 1000} 秒</Fact><Fact label="时长依据">{shot.timing_basis}</Fact><Fact label="进入状态">{shot.entry_state}</Fact><Fact label="离开状态">{shot.exit_state}</Fact><Fact label="画内人物与物体">{shot.visible_mentions.map((key) => scene?.mentions.find((m) => m.key === key)?.name ?? key).join("、")}</Fact><Fact label="覆盖节拍">{shot.beat_keys.map((key) => scene?.beats.find((b) => b.key === key)?.action ?? key).join("；")}</Fact></Facts>
      <div><h4 className="text-sm font-medium">声音与对白</h4>{shot.audio.length === 0 ? <p className="mt-1 text-sm text-muted-foreground">本镜无对白</p> : shot.audio.map((audio, index) => <p className="mt-1 whitespace-pre-wrap text-sm" key={index}>{channelLabels[audio.channel]}：{scene?.dialogues.find((d) => d.key === audio.dialogue_key)?.text ?? audio.dialogue_key}</p>)}</div><Quotes items={shot.detail_evidence} />
    </section>)}
  </div>;
}

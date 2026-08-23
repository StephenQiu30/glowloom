"use client";

import type { FormEvent } from "react";

type Operation = API.NarrativeOperation;
type OperationInput = Omit<Partial<Operation>, "type"> & { type: API.NarrativeOperationType };

type NarrativeEditorProps = {
  analysis: API.Analysis;
  disabled: boolean;
  editable: boolean;
  onRevise: (operations: Operation[]) => Promise<void>;
};

function command(input: OperationInput): Operation {
  return {
    anchor: null,
    boundary_node_id: null,
    element_type: null,
    episode_key: null,
    heading: null,
    ignore_reason: null,
    left_heading: null,
    left_scene_id: null,
    mention_id: null,
    node_id: null,
    node_kind: null,
    ordered_node_ids: null,
    ordered_scene_ids: null,
    right_heading: null,
    right_scene_id: null,
    scene_id: null,
    scene_ids: null,
    speaker: null,
    surface_text: null,
    target_scene_id: null,
    text: null,
    ...input,
  };
}

function textValue(form: HTMLFormElement, name: string) {
  return String(new FormData(form).get(name) ?? "").trim();
}

function numberValue(form: HTMLFormElement, name: string) {
  return Number(new FormData(form).get(name));
}

function newMemberID() {
  return globalThis.crypto.randomUUID();
}

function nodeAnchor(form: HTMLFormElement, line: number | null | undefined): API.Anchor {
  return {
    line: line ?? 0,
    start_offset: numberValue(form, "start_offset"),
    end_offset: numberValue(form, "end_offset"),
  };
}

export function NarrativeEditor({ analysis, disabled, editable, onRevise }: NarrativeEditorProps) {
  const narrative = analysis.narrative;
  if (!narrative?.id) return null;
  const episodes = (analysis.episodes ?? []).filter((episode) => episode.decision !== "ignored");
  const mentions = analysis.mentions ?? [];

  function submit(event: FormEvent<HTMLFormElement>, operation: Operation) {
    event.preventDefault();
    void onRevise([operation]);
  }

  function reorderScenes(episode: API.Episode, sceneIndex: number, direction: -1 | 1) {
    const scenes = episode.scenes ?? [];
    const target = sceneIndex + direction;
    if (target < 0 || target >= scenes.length) return;
    const ids = scenes.map((scene) => scene.id ?? "");
    [ids[sceneIndex], ids[target]] = [ids[target], ids[sceneIndex]];
    void onRevise([command({ type: "reorder_scenes", episode_key: episode.temporary_key, ordered_scene_ids: ids })]);
  }

  function reorderNodes(scene: API.Scene, nodeIndex: number, direction: -1 | 1) {
    const nodes = scene.narratives ?? [];
    const target = nodeIndex + direction;
    if (target < 0 || target >= nodes.length) return;
    const ids = nodes.map((node) => node.id ?? "");
    [ids[nodeIndex], ids[target]] = [ids[target], ids[nodeIndex]];
    void onRevise([command({ type: "reorder_nodes", scene_id: scene.id, ordered_node_ids: ids })]);
  }

  return (
    <section className="card narrative-card" aria-labelledby="narrative-title">
      <div className="section-heading">
        <div>
          <h2 id="narrative-title">4. 校对叙事与 Mention</h2>
          <p>每次结构或 Mention 操作都会创建新的不可变 NarrativeRevision；批准前不会进入 M04/M05。</p>
        </div>
        <span className={`breakdown-state ${narrative.status === "ready" ? "ready" : "blocked"}`}>
          {narrative.status === "ready" ? "叙事校验通过" : narrative.status === "approved" ? "叙事已批准" : "叙事校验阻塞"}
        </span>
      </div>
      <div className="breakdown-basis">
        <span>Revision {narrative.revision_no ?? "—"}</span>
        <span>Content {narrative.content_hash?.slice(0, 12) ?? "—"}</span>
        <span>Completeness {narrative.completeness ?? "—"}</span>
      </div>
      {(narrative.issues?.length ?? 0) > 0 && <div className="breakdown-issues" role="alert">
        <strong>批准前必须解决：</strong>
        <ul>
          {(narrative.issues ?? []).map((issue, index) => <li key={`${issue.code}-${issue.scene_id}-${issue.node_id}-${index}`}>
            {issue.message ?? issue.code}
            {issue.anchor && <span className="hint"> · Offset {issue.anchor.start_offset}—{issue.anchor.end_offset}</span>}
          </li>)}
        </ul>
      </div>}

      <div className="narrative-episodes">
        {episodes.map((episode, episodeIndex) => <article className="narrative-episode" key={episode.temporary_key ?? episode.content_unit_id ?? episodeIndex}>
          <div className="episode-head">
            <span className="episode-title">第 {episode.number} 集 · {episode.title}</span>
            <span className="hint">ContentUnit {episode.content_unit_id?.slice(0, 8) ?? "未物化"}</span>
          </div>
          <div className="narrative-scenes">
            {(episode.scenes ?? []).map((scene, sceneIndex) => {
              const sceneNumber = sceneIndex + 1;
              const sceneLabel = `第 ${episode.number} 集场景 ${sceneNumber}`;
              const nextScene = episode.scenes?.[sceneIndex + 1];
              const sceneMentions = mentions.filter((mention) => mention.scene_id === scene.id);
              const nodes = scene.narratives ?? [];
              return <section className="narrative-scene" aria-label={sceneLabel} key={scene.id ?? sceneIndex}>
                <div className="episode-head">
                  <strong>{sceneLabel} · {scene.heading}</strong>
                  <span className="hint">Offset {scene.anchor?.start_offset}—{scene.anchor?.end_offset}</span>
                </div>

                {editable && <div className="narrative-controls">
                  <form onSubmit={(event) => submit(event, command({
                    type: "update_scene", scene_id: scene.id, heading: textValue(event.currentTarget, "heading"),
                  }))}>
                    <label htmlFor={`episode-${episodeIndex}-scene-${sceneIndex}-heading`}>{sceneLabel} 标题</label>
                    <input id={`episode-${episodeIndex}-scene-${sceneIndex}-heading`} name="heading" defaultValue={scene.heading ?? ""} required maxLength={200} />
                    <button className="secondary" type="submit" disabled={disabled}>保存{sceneLabel} 标题</button>
                  </form>
                  <div className="actions">
                    <button className="secondary" type="button" disabled={disabled || sceneIndex === 0} onClick={() => reorderScenes(episode, sceneIndex, -1)}>上移{sceneLabel}</button>
                    <button className="secondary" type="button" disabled={disabled || sceneIndex === (episode.scenes?.length ?? 0) - 1} onClick={() => reorderScenes(episode, sceneIndex, 1)}>下移{sceneLabel}</button>
                  </div>
                  {nodes.length > 1 && <form onSubmit={(event) => submit(event, command({
                    type: "split_scene", scene_id: scene.id,
                    boundary_node_id: textValue(event.currentTarget, "boundary_node_id"),
                    left_scene_id: newMemberID(), right_scene_id: newMemberID(),
                    left_heading: textValue(event.currentTarget, "left_heading"),
                    right_heading: textValue(event.currentTarget, "right_heading"),
                  }))}>
                    <label htmlFor={`scene-${scene.id}-split`}>{sceneLabel}拆分边界</label>
                    <select id={`scene-${scene.id}-split`} name="boundary_node_id" defaultValue={nodes[1].id ?? ""}>
                      {nodes.slice(1).map((node, nodeIndex) => <option key={node.id ?? nodeIndex} value={node.id ?? ""}>节点 {nodeIndex + 2} · {node.text}</option>)}
                    </select>
                    <div className="breakdown-columns">
                      <div><label htmlFor={`scene-${scene.id}-left-heading`}>拆分后左侧标题</label><input id={`scene-${scene.id}-left-heading`} name="left_heading" defaultValue={`${scene.heading ?? sceneLabel}（上）`} required /></div>
                      <div><label htmlFor={`scene-${scene.id}-right-heading`}>拆分后右侧标题</label><input id={`scene-${scene.id}-right-heading`} name="right_heading" defaultValue={`${scene.heading ?? sceneLabel}（下）`} required /></div>
                    </div>
                    <button className="secondary" type="submit" disabled={disabled}>拆分{sceneLabel}</button>
                  </form>}
                  {nextScene && <form onSubmit={(event) => submit(event, command({
                    type: "merge_scenes", scene_ids: [scene.id ?? "", nextScene.id ?? ""],
                    target_scene_id: newMemberID(), heading: textValue(event.currentTarget, "heading"),
                  }))}>
                    <label htmlFor={`scene-${scene.id}-merge-heading`}>{sceneLabel}与下一场合并标题</label>
                    <input id={`scene-${scene.id}-merge-heading`} name="heading" defaultValue={`${scene.heading ?? ""} / ${nextScene.heading ?? ""}`} required />
                    <button className="secondary" type="submit" disabled={disabled}>合并{sceneLabel}与下一场</button>
                  </form>}
                </div>}

                <div className="narrative-nodes">
                  {nodes.map((node, nodeIndex) => {
                    const nodeNumber = nodeIndex + 1;
                    const nodeLabel = `${sceneLabel}节点 ${nodeNumber}`;
                    return <article className={`narrative-node ${node.status === "ignored" ? "ignored" : ""}`} key={node.id ?? nodeIndex}>
                      {editable ? <form onSubmit={(event) => submit(event, command({
                        type: "update_node", node_id: node.id,
                        node_kind: textValue(event.currentTarget, "kind") as API.NarrativeNodeKind,
                        text: textValue(event.currentTarget, "text"), speaker: textValue(event.currentTarget, "speaker"),
                        anchor: nodeAnchor(event.currentTarget, node.anchor?.line),
                      }))}>
                        <div className="breakdown-columns">
                          <div><label htmlFor={`node-${node.id}-kind`}>{nodeLabel} 类型</label><select id={`node-${node.id}-kind`} name="kind" defaultValue={node.kind ?? "action"}><option value="beat">Beat</option><option value="dialogue">Dialogue</option><option value="action">Action</option><option value="narration">Narration</option></select></div>
                          <div><label htmlFor={`node-${node.id}-speaker`}>{nodeLabel} 说话人</label><input id={`node-${node.id}-speaker`} name="speaker" defaultValue={node.speaker ?? ""} /></div>
                        </div>
                        <label htmlFor={`node-${node.id}-text`}>{nodeLabel} 正文</label>
                        <textarea className="compact-textarea" id={`node-${node.id}-text`} name="text" defaultValue={node.text ?? ""} required />
                        <div className="breakdown-columns">
                          <div><label htmlFor={`node-${node.id}-start`}>{nodeLabel} 起始 Offset</label><input id={`node-${node.id}-start`} name="start_offset" type="number" defaultValue={node.anchor?.start_offset ?? 0} required /></div>
                          <div><label htmlFor={`node-${node.id}-end`}>{nodeLabel} 结束 Offset</label><input id={`node-${node.id}-end`} name="end_offset" type="number" defaultValue={node.anchor?.end_offset ?? 0} required /></div>
                        </div>
                        <button className="secondary" type="submit" disabled={disabled}>保存{nodeLabel}</button>
                      </form> : <p><strong>{node.kind}</strong> · {node.text}</p>}
                      {editable && <div className="node-actions">
                        <button className="secondary" type="button" disabled={disabled || nodeIndex === 0} onClick={() => reorderNodes(scene, nodeIndex, -1)}>上移{nodeLabel}</button>
                        <button className="secondary" type="button" disabled={disabled || nodeIndex === nodes.length - 1} onClick={() => reorderNodes(scene, nodeIndex, 1)}>下移{nodeLabel}</button>
                        <button className="secondary" type="button" disabled={disabled} onClick={() => void onRevise([command({ type: "delete_node", node_id: node.id })])}>删除{nodeLabel}</button>
                      </div>}
                      {editable && <form onSubmit={(event) => submit(event, command({ type: "ignore_node", node_id: node.id, ignore_reason: textValue(event.currentTarget, "ignore_reason") }))}>
                        <label htmlFor={`node-${node.id}-ignore`}>{nodeLabel} 忽略理由</label>
                        <input id={`node-${node.id}-ignore`} name="ignore_reason" defaultValue={node.ignore_reason ?? "不进入批准叙事"} required />
                        <button className="secondary" type="submit" disabled={disabled}>具名忽略{nodeLabel}</button>
                      </form>}
                    </article>;
                  })}
                </div>

                {editable && <form className="narrative-create" onSubmit={(event) => submit(event, command({
                  type: "create_node", node_id: newMemberID(), scene_id: scene.id,
                  node_kind: textValue(event.currentTarget, "kind") as API.NarrativeNodeKind,
                  text: textValue(event.currentTarget, "text"), speaker: textValue(event.currentTarget, "speaker"),
                  anchor: nodeAnchor(event.currentTarget, scene.anchor?.line),
                }))}>
                  <strong>在{sceneLabel}创建叙事节点</strong>
                  <div className="breakdown-columns">
                    <div><label htmlFor={`scene-${scene.id}-new-node-kind`}>新节点类型</label><select id={`scene-${scene.id}-new-node-kind`} name="kind" defaultValue="action"><option value="beat">Beat</option><option value="dialogue">Dialogue</option><option value="action">Action</option><option value="narration">Narration</option></select></div>
                    <div><label htmlFor={`scene-${scene.id}-new-node-speaker`}>新节点说话人</label><input id={`scene-${scene.id}-new-node-speaker`} name="speaker" /></div>
                  </div>
                  <label htmlFor={`scene-${scene.id}-new-node-text`}>新节点正文</label><input id={`scene-${scene.id}-new-node-text`} name="text" required />
                  <div className="breakdown-columns">
                    <div><label htmlFor={`scene-${scene.id}-new-node-start`}>新节点起始 Offset</label><input id={`scene-${scene.id}-new-node-start`} name="start_offset" type="number" defaultValue={scene.anchor?.start_offset ?? 0} required /></div>
                    <div><label htmlFor={`scene-${scene.id}-new-node-end`}>新节点结束 Offset</label><input id={`scene-${scene.id}-new-node-end`} name="end_offset" type="number" defaultValue={scene.anchor?.end_offset ?? 0} required /></div>
                  </div>
                  <button className="secondary" type="submit" disabled={disabled}>创建叙事节点</button>
                </form>}

                <div className="mention-list" aria-label={`${sceneLabel} Mention`}>
                  {sceneMentions.map((mention) => <div className="mention-row" key={mention.id ?? `${mention.element_type}-${mention.surface_text}`}>
                    {editable ? <form onSubmit={(event) => submit(event, command({
                      type: "update_mention", mention_id: mention.id, scene_id: scene.id,
                      element_type: textValue(event.currentTarget, "element_type"), surface_text: textValue(event.currentTarget, "surface_text"),
                      anchor: nodeAnchor(event.currentTarget, mention.anchor?.line),
                    }))}>
                      <div className="breakdown-columns">
                        <div><label htmlFor={`mention-${mention.id}-type`}>Mention {mention.surface_text} 类型</label><select id={`mention-${mention.id}-type`} name="element_type" defaultValue={mention.element_type ?? "character"}><option value="character">人物</option><option value="location">地点</option><option value="prop">道具</option><option value="costume">服装</option></select></div>
                        <div><label htmlFor={`mention-${mention.id}-text`}>Mention {mention.surface_text} 来源文本</label><input id={`mention-${mention.id}-text`} name="surface_text" defaultValue={mention.surface_text ?? ""} required /></div>
                      </div>
                      <div className="breakdown-columns">
                        <div><label htmlFor={`mention-${mention.id}-start`}>Mention {mention.surface_text} 起始 Offset</label><input id={`mention-${mention.id}-start`} name="start_offset" type="number" defaultValue={mention.anchor?.start_offset ?? 0} required /></div>
                        <div><label htmlFor={`mention-${mention.id}-end`}>Mention {mention.surface_text} 结束 Offset</label><input id={`mention-${mention.id}-end`} name="end_offset" type="number" defaultValue={mention.anchor?.end_offset ?? 0} required /></div>
                      </div>
                      <div className="node-actions"><button className="secondary" type="submit" disabled={disabled}>保存 Mention {mention.surface_text}</button><button className="secondary" type="button" disabled={disabled} onClick={() => void onRevise([command({ type: "delete_mention", mention_id: mention.id })])}>删除 Mention {mention.surface_text}</button></div>
                    </form> : <span>{mention.surface_text} · {mention.element_type}</span>}
                  </div>)}
                </div>

                {editable && <form className="narrative-create" onSubmit={(event) => submit(event, command({
                  type: "create_mention", mention_id: newMemberID(), scene_id: scene.id,
                  element_type: textValue(event.currentTarget, "element_type"), surface_text: textValue(event.currentTarget, "surface_text"),
                  anchor: nodeAnchor(event.currentTarget, scene.anchor?.line),
                }))}>
                  <strong>在{sceneLabel}创建 Mention</strong>
                  <div className="breakdown-columns">
                    <div><label htmlFor={`scene-${scene.id}-new-mention-type`}>场景 {sceneNumber} 新 Mention 类型</label><select id={`scene-${scene.id}-new-mention-type`} name="element_type" defaultValue="character"><option value="character">人物</option><option value="location">地点</option><option value="prop">道具</option><option value="costume">服装</option></select></div>
                    <div><label htmlFor={`scene-${scene.id}-new-mention-text`}>场景 {sceneNumber} 新 Mention 来源文本</label><input id={`scene-${scene.id}-new-mention-text`} name="surface_text" required /></div>
                  </div>
                  <div className="breakdown-columns">
                    <div><label htmlFor={`scene-${scene.id}-new-mention-start`}>场景 {sceneNumber} 新 Mention 起始 Offset</label><input id={`scene-${scene.id}-new-mention-start`} name="start_offset" type="number" defaultValue={scene.anchor?.start_offset ?? 0} required /></div>
                    <div><label htmlFor={`scene-${scene.id}-new-mention-end`}>场景 {sceneNumber} 新 Mention 结束 Offset</label><input id={`scene-${scene.id}-new-mention-end`} name="end_offset" type="number" defaultValue={scene.anchor?.end_offset ?? 0} required /></div>
                  </div>
                  <button className="secondary" type="submit" disabled={disabled}>在场景 {sceneNumber} 创建 Mention</button>
                </form>}
              </section>;
            })}
          </div>
        </article>)}
      </div>
    </section>
  );
}

"use client";

import type { FormEvent } from "react";

type Analysis = API.Analysis;
type Episode = API.Episode;
type Operation = API.EpisodeBreakdownOperation;
type OperationInput = Omit<Partial<Operation>, "type"> & { type: API.BreakdownOperationType };

type EpisodeBreakdownEditorProps = {
  analysis: Analysis;
  disabled: boolean;
  editable: boolean;
  onRevise: (operations: Operation[]) => Promise<void>;
};

function requiredCandidateKey(episode: Episode) {
  return episode.temporary_key ?? "";
}

function manualCandidateKey(episode: Episode, suffix: string, revisionNo: number) {
  const base = requiredCandidateKey(episode).replace(/[^a-zA-Z0-9_-]/g, "-").slice(-48);
  return `${base}-${suffix}-r${revisionNo}`.slice(0, 120);
}

function formText(form: HTMLFormElement, name: string) {
  return String(new FormData(form).get(name) ?? "").trim();
}

function formNumber(form: HTMLFormElement, name: string) {
  return Number(new FormData(form).get(name));
}

function command(input: OperationInput): Operation {
  return {
    boundary_offset: null,
    candidate_key: null,
    candidate_keys: null,
    left_key: null,
    left_title: null,
    ordered_candidate_keys: null,
    right_key: null,
    right_title: null,
    target_key: null,
    target_title: null,
    title: null,
    ...input,
  };
}

export function EpisodeBreakdownEditor({ analysis, disabled, editable, onRevise }: EpisodeBreakdownEditorProps) {
  const breakdown = analysis.breakdown;
  const episodes = analysis.episodes ?? [];
  if (!breakdown) return null;
  const nextRevisionNo = (breakdown.revision_no ?? 0) + 1;

  function submit(event: FormEvent<HTMLFormElement>, operation: Operation) {
    event.preventDefault();
    void onRevise([operation]);
  }

  function reorder(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= episodes.length) return;
    const keys = episodes.map(requiredCandidateKey);
    [keys[index], keys[target]] = [keys[target], keys[index]];
    void onRevise([command({ type: "reorder", ordered_candidate_keys: keys })]);
  }

  return (
    <section className="card breakdown-card" aria-labelledby="breakdown-title">
      <div className="section-heading">
        <div>
          <h2 id="breakdown-title">3. 校对剧集边界</h2>
          <p>只允许在完整场次之间拆分或移动边界。每次操作创建新的不可变拆解修订，来源 hash 不变。</p>
        </div>
        <span className={`breakdown-state ${breakdown.status === "ready" ? "ready" : "blocked"}`}>
          {breakdown.status === "ready" ? "覆盖校验通过" : "覆盖校验阻塞"}
        </span>
      </div>
      <div className="breakdown-basis">
        <span>Revision {breakdown.revision_no ?? "—"}</span>
        <span>Coverage {breakdown.coverage_hash?.slice(0, 12) ?? "—"}</span>
        <span>Segmentation {breakdown.segmentation_hash?.slice(0, 12) ?? "—"}</span>
      </div>
      {(breakdown.issues?.length ?? 0) > 0 && <div className="breakdown-issues" role="alert">
        <strong>批准前必须解决：</strong>
        <ul>
          {(breakdown.issues ?? []).map((issue, index) => <li key={`${issue.code}-${index}`}>
            {issue.message ?? issue.code}
            {issue.anchor && <span className="hint"> · Offset {issue.anchor.start_offset}—{issue.anchor.end_offset}</span>}
          </li>)}
        </ul>
      </div>}

      <div className="breakdown-list">
        {episodes.map((episode, index) => {
          const range = index + 1;
          const next = episodes[index + 1];
          const splitBoundaries = (episode.scenes ?? []).slice(1).filter((scene) => scene.anchor?.start_offset != null);
          const moveBoundaries = next
            ? [...(episode.scenes ?? []), ...(next.scenes ?? [])]
              .filter((scene) => {
                const offset = scene.anchor?.start_offset;
                return offset != null && offset > (episode.anchor?.start_offset ?? -1) && offset < (next.anchor?.end_offset ?? Number.MAX_SAFE_INTEGER);
              })
            : [];
          return <article className={`breakdown-range ${episode.decision === "ignored" ? "ignored" : ""}`} key={requiredCandidateKey(episode) || range}>
            <div className="episode-head">
              <div>
                <span className="episode-title">范围 {range} · {episode.title}</span>
                <div className="hint">{episode.temporary_key} · Offset {episode.anchor?.start_offset}—{episode.anchor?.end_offset} · {episode.boundary_rule}</div>
              </div>
              <span className="hint">{episode.decision === "ignored" ? "已具名忽略" : `${episode.scenes?.length ?? 0} 个完整场次`}</span>
            </div>

            {editable && <div className="breakdown-actions">
              <form onSubmit={(event) => submit(event, command({
                type: "rename",
                candidate_key: requiredCandidateKey(episode),
                title: formText(event.currentTarget, "title"),
              }))}>
                <label htmlFor={`range-${range}-title`}>范围 {range} 标题</label>
                <input id={`range-${range}-title`} name="title" defaultValue={episode.title ?? ""} required maxLength={200} />
                <button className="secondary" type="submit" disabled={disabled}>保存范围 {range} 标题</button>
              </form>

              <div className="actions">
                <button className="secondary" type="button" onClick={() => reorder(index, -1)} disabled={disabled || index === 0}>上移范围 {range}</button>
                <button className="secondary" type="button" onClick={() => reorder(index, 1)} disabled={disabled || index === episodes.length - 1}>下移范围 {range}</button>
              </div>

              {splitBoundaries.length > 0 && <form onSubmit={(event) => submit(event, command({
                type: "split",
                candidate_key: requiredCandidateKey(episode),
                boundary_offset: formNumber(event.currentTarget, "boundary"),
                left_key: manualCandidateKey(episode, "left", nextRevisionNo),
                right_key: manualCandidateKey(episode, "right", nextRevisionNo),
                left_title: formText(event.currentTarget, "left_title"),
                right_title: formText(event.currentTarget, "right_title"),
              }))}>
                <label htmlFor={`range-${range}-split`}>范围 {range} 拆分边界</label>
                <select id={`range-${range}-split`} name="boundary" defaultValue={String(splitBoundaries[0].anchor?.start_offset)}>
                  {splitBoundaries.map((scene) => <option value={String(scene.anchor?.start_offset)} key={scene.id ?? scene.anchor?.start_offset ?? scene.heading ?? "scene"}>
                    Offset {scene.anchor?.start_offset} · {scene.heading}
                  </option>)}
                </select>
                <div className="breakdown-columns">
                  <div><label htmlFor={`range-${range}-left-title`}>范围 {range} 拆分后左侧标题</label><input id={`range-${range}-left-title`} name="left_title" defaultValue={`${episode.title ?? `范围 ${range}`}（上）`} required maxLength={200} /></div>
                  <div><label htmlFor={`range-${range}-right-title`}>范围 {range} 拆分后右侧标题</label><input id={`range-${range}-right-title`} name="right_title" defaultValue={`${episode.title ?? `范围 ${range}`}（下）`} required maxLength={200} /></div>
                </div>
                <button className="secondary" type="submit" disabled={disabled}>拆分范围 {range}</button>
              </form>}

              <form onSubmit={(event) => submit(event, command({
                type: "ignore",
                candidate_key: requiredCandidateKey(episode),
                title: formText(event.currentTarget, "reason"),
              }))}>
                <label htmlFor={`range-${range}-ignore`}>范围 {range} 忽略理由</label>
                <input id={`range-${range}-ignore`} name="reason" defaultValue={episode.decision === "ignored" ? episode.title ?? "" : "非正片来源范围"} required maxLength={200} />
                <button className="secondary" type="submit" disabled={disabled}>具名忽略范围 {range}</button>
              </form>
            </div>}

            {editable && next && <div className="boundary-bridge">
              <form onSubmit={(event) => submit(event, command({
                type: "merge",
                candidate_keys: [requiredCandidateKey(episode), requiredCandidateKey(next)],
                target_key: manualCandidateKey(episode, "merged", nextRevisionNo),
                target_title: formText(event.currentTarget, "target_title"),
              }))}>
                <label htmlFor={`range-${range}-merge-title`}>范围 {range} 与 {range + 1} 合并标题</label>
                <input id={`range-${range}-merge-title`} name="target_title" defaultValue={`${episode.title ?? ""} / ${next.title ?? ""}`} required maxLength={200} />
                <button className="secondary" type="submit" disabled={disabled}>合并范围 {range} 与 {range + 1}</button>
              </form>
              {moveBoundaries.length > 0 && <form onSubmit={(event) => submit(event, command({
                type: "move_boundary",
                left_key: requiredCandidateKey(episode),
                right_key: requiredCandidateKey(next),
                boundary_offset: formNumber(event.currentTarget, "boundary"),
              }))}>
                <label htmlFor={`range-${range}-move-boundary`}>范围 {range} 与 {range + 1} 边界</label>
                <select id={`range-${range}-move-boundary`} name="boundary" defaultValue={String(next.anchor?.start_offset)}>
                  {moveBoundaries.map((scene) => <option value={String(scene.anchor?.start_offset)} key={`${scene.id}-${scene.anchor?.start_offset}`}>
                    Offset {scene.anchor?.start_offset} · {scene.heading}
                  </option>)}
                </select>
                <button className="secondary" type="submit" disabled={disabled}>移动范围 {range} 与 {range + 1} 边界</button>
              </form>}
            </div>}
          </article>;
        })}
      </div>
    </section>
  );
}

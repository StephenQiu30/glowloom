type ShotContent = "id" | "revision" | "title" | "action" | "duration_ms";
export type CanvasStoryboardShot = API.StoryboardShot &
  Required<Pick<API.StoryboardShot, ShotContent>> & {
    adopted?: Required<API.StoryboardTake>;
  };

export type StoryboardField =
  "title" | "action" | "duration_ms" | "dialogue" | "camera" | "sound";
export type ShotDraft = {
  base: string | number;
  value: string;
};

export function storyboardMove(
  shots: readonly CanvasStoryboardShot[],
  shotID: string,
  step: -1 | 1,
) {
  const index = shots.findIndex((shot) => shot.id === shotID);
  const target = index + step;
  if (index < 0 || target < 0 || target >= shots.length) return null;
  const before = shots[step < 0 ? target : target + 1]?.id;
  return { shot_id: shotID, ...(before ? { before_id: before } : {}) };
}

export function storyboardFieldValue(field: StoryboardField, text: string) {
  if (field !== "duration_ms") return text.trim();
  const milliseconds = Number(text) * 1000;
  return Number.isFinite(milliseconds) ? Math.round(milliseconds) : NaN;
}

export function storyboardEdit(
  shot: CanvasStoryboardShot,
  field: StoryboardField,
  draft: ShotDraft,
) {
  const committed = shot[field] ?? "";
  if (draft.base !== committed) return null;
  const value = storyboardFieldValue(field, draft.value);
  if (
    (!value &&
      (field === "title" || field === "action" || field === "duration_ms")) ||
    (typeof value === "number" && !Number.isSafeInteger(value)) ||
    (typeof value === "number" && value < 1) ||
    value === committed
  )
    return null;
  return { shot_id: shot.id, expected_revision: shot.revision, field, value };
}

export function shotReference(
  boardID: string,
  shot: CanvasStoryboardShot,
  ordinal: number,
) {
  return `参考第 ${ordinal} 镜（board_id=${boardID}, shot_id=${shot.id}, shot_revision=${shot.revision}）「${shot.title.replace(/\s+/g, " ")}」。动作：${shot.action.replace(/\s+/g, " ")}。计划时长：${shot.duration_ms / 1000} 秒。`;
}

export function storyboardReferenceLabel(text: string) {
  const match =
    /^参考第 (\d{1,2}) 镜（board_id=([0-9a-f-]{36}), shot_id=([0-9a-f-]{36}), shot_revision=\d+）/i.exec(
      text,
    );
  return match
    ? { key: `${match[2]}/${match[3]}`, label: `第 ${match[1]} 镜` }
    : undefined;
}

export type StoryboardActionProposal = {
  target: API.StoryboardTargetDto;
  action: string;
};

export function storyboardActionProposal(
  text: string,
  input?: API.AuthorizedInput,
): StoryboardActionProposal | undefined {
  const [target, extra] = input?.storyboard_targets ?? [];
  const [proposal, another] = text.matchAll(/^动作建议：[ \t]*([^\r\n]+)$/gm);
  const action = proposal?.[1]?.trim();
  if (!target || extra || another || !action || [...action].length > 2000)
    return;
  return { target, action };
}

export function storyboardDirections(text: string) {
  return [
    ...new Set(
      Array.from(
        text.matchAll(/^(?:#{2,4} )?方向：[ \t]*([^\r\n]{1,60})$/gm),
        (match) => match[1]!.trim(),
      ).filter(Boolean),
    ),
  ].slice(0, 4);
}

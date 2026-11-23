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
  return `参考第 ${ordinal} 镜（board_id=${boardID}, shot_id=${shot.id}, shot_revision=${shot.revision}）「${shot.title}」。动作：${shot.action}。计划时长：${shot.duration_ms / 1000} 秒。`;
}

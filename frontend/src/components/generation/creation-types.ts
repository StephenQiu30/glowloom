import type { PromptDraft } from "@/components/generation/prompt-document";

export const creationTypes = [
  {
    id: "agent",
    label: "Agent 模式",
    placeholder: "描述图片创作目标，或提出修改要求。",
  },
  {
    id: "image",
    label: "图片生成",
    placeholder: "描述你的想法，或添加参考素材。",
  },
  {
    id: "video",
    label: "视频生成",
    placeholder: "描述画面、动作与镜头，或添加参考素材。",
  },
  { id: "music", label: "音乐生成", placeholder: "描述曲风、情绪与节奏。" },
  {
    id: "audio",
    label: "音频生成",
    placeholder: "输入需要朗读的文字。",
  },
  {
    id: "avatar",
    label: "数字人",
    placeholder: "描述表情、姿态与动作（可选）。",
  },
  {
    id: "motion",
    label: "动作模仿",
    placeholder: "描述你希望主体模仿的动作。",
  },
] as const;
export type CreationType = (typeof creationTypes)[number]["id"];

export function executableCreationType(
  type: CreationType,
): type is Exclude<CreationType, "agent"> {
  return type !== "agent";
}

export function creationCapability(
  type: Exclude<CreationType, "agent">,
): "image" | "video" | "audio" {
  return type === "music"
    ? "audio"
    : type === "avatar" || type === "motion"
      ? "video"
      : type;
}

export function creationComposer(
  type: CreationType,
  value: PromptDraft,
): PromptDraft {
  const draft = structuredClone(value);
  if (type === "music") {
    draft.mode =
      draft.params.music_mode === "instrumental"
        ? "music_instrumental"
        : "music_song";
    delete draft.params.music_mode;
    delete draft.params.music_auto_duration;
    draft.params.duration_ms ??= 60000;
  }
  if (type === "avatar" || type === "motion") {
    draft.mode = type;
    if (
      type === "avatar" &&
      typeof draft.params.avatar_action === "string" &&
      draft.params.avatar_action
    )
      draft.document.content = [
        {
          type: "paragraph",
          content: [{ type: "text", text: draft.params.avatar_action }],
        },
      ];
    delete draft.params.avatar_action;
  }
  return draft;
}

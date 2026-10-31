import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { requireTask } from "@/components/generation/contracts";

export const assistantSkills = [
  {
    id: "video_analysis",
    title: "视频反解",
    description: "拆解参考视频的镜头与光影，形成用于复刻和再创作的视频提示词",
  },
  {
    id: "storyboard",
    title: "创作分镜",
    description: "将创意整理为电影级分镜提示词，结合运镜与导演风格",
  },
  {
    id: "advertisement",
    title: "全流程广告片导演",
    description: "从广告需求到创意、镜头与配乐的完整广告片",
  },
  {
    id: "short_film",
    title: "剧情短片",
    description: "创意、剧本、素材、分镜到视频的完整创作流程",
  },
  {
    id: "product_images",
    title: "电商套图",
    description: "生成风格统一的商品视觉素材，适配电商展示",
  },
  {
    id: "poster",
    title: "海报设计",
    description: "面向营销场景和节日主题的海报创作",
  },
  {
    id: "brand",
    title: "品牌设计",
    description: "根据公司、业务与客群形成品牌标志和视觉方案",
  },
] as const;
export type AssistantSkill = (typeof assistantSkills)[number]["id"] | "";
export type AssistantSession = Omit<API.AssistantSession, "turns"> & {
  id: string;
  canvas_id: string;
  project_id: string;
  revision: number;
  title: string;
  turns: (API.AssistantTurn & { draft_id: string })[];
};
export function requireAssistantSession(
  value: API.AssistantSession,
  canvasID: string,
  projectID: string,
): AssistantSession {
  if (
    !value ||
    !isProjectID(value.id) ||
    value.canvas_id !== canvasID ||
    value.project_id !== projectID ||
    !Number.isSafeInteger(value.revision) ||
    value.revision! < 1 ||
    typeof value.title !== "string" ||
    !Array.isArray(value.turns) ||
    value.turns.length > 64 ||
    value.turns.some(
      (t) =>
        !isProjectID(t.draft_id) ||
        (t.operation_id !== undefined && !isProjectID(t.operation_id)) ||
        (t.execution !== undefined &&
          !["discussion", "image"].includes(t.execution)) ||
        (t.execution === "image" &&
          !["poster", "product_images"].includes(t.skill ?? "")) ||
        (t.skill && !assistantSkills.some((s) => s.id === t.skill)),
    ) ||
    new Set(value.turns.map((t) => t.draft_id)).size !== value.turns.length
  )
    throw new RequestError("invalid_receipt");
  return value as AssistantSession;
}
/** The frozen editor already contains earlier context; append only its own reply. */
export function assistantContinuation(
  value: API.TaskDetail,
  projectID: string,
  operationID: string,
  media: readonly API.AssetSummary[] = [],
): PromptDraft {
  const task = requireTask(value, projectID);
  if (
    task.id !== operationID ||
    !["completed", "failed", "cancelled", "expired"].includes(task.status) ||
    !task.editor_draft
  )
    throw new RequestError("canvas_assistant_turn_active");
  validatePromptDraft(task.editor_draft);
  const draft: PromptDraft = structuredClone(task.editor_draft);
  if (task.status === "completed") {
    const replies = task.outputs?.filter(
      (o) =>
        o.kind === "json" &&
        o.moderation_status === "passed" &&
        typeof o.text === "string" &&
        o.text.trim(),
    );
    const images =
      task.outputs?.filter(
        (o) =>
          o.kind === "image" &&
          o.moderation_status === "passed" &&
          o.media_asset_id,
      ) ?? [];
    if (!replies?.length && !images.length)
      throw new RequestError("assistant_reply_unavailable");
    draft.document.content.push(
      { type: "paragraph", content: [{ type: "text", text: "助手：" }] },
      ...(replies ?? []).flatMap((o) =>
        o.text!.split("\n").map((text) => ({
          type: "paragraph" as const,
          ...(text ? { content: [{ type: "text" as const, text }] } : {}),
        })),
      ),
    );
  }
  if (task.status === "completed")
    for (const output of task.outputs?.filter(
      (o) =>
        o.kind === "image" &&
        o.moderation_status === "passed" &&
        o.media_asset_id,
    ) ?? []) {
      const asset = media.find((a) => a.id === output.media_asset_id);
      if (
        !asset ||
        asset.project_id !== projectID ||
        asset.kind !== "image" ||
        !Number.isSafeInteger(asset.revision) ||
        asset.revision! < 1
      )
        throw new RequestError("assistant_reply_unavailable");
      let reference = draft.references.find(
        (r) =>
          r.source.kind === "media" &&
          r.source.sourceId === asset.id &&
          r.source.sourceVersion === String(asset.revision),
      );
      if (!reference) {
        reference = {
          id: crypto.randomUUID(),
          bindingGeneration: 1,
          mediaKind: "image",
          role: "reference_image",
          order: draft.references.length,
          source: {
            kind: "media",
            sourceId: asset.id!,
            sourceVersion: String(asset.revision),
          },
        };
        draft.references.push(reference);
        draft.labelState.allocations[reference.id] = {
          labelKind: "image",
          ordinal: ++draft.labelState.highWater.image,
        };
      }
      draft.document.content.push({
        type: "paragraph",
        content: [
          { type: "text", text: "已生成图片：" },
          { type: "reference", attrs: { referenceId: reference.id } },
        ],
      });
    }
  if (
    !task.capability?.startsWith("text.") &&
    !task.capability?.startsWith("document.")
  ) {
    draft.mode = null;
    draft.modelVersion = null;
    draft.params = { output_count: 1 };
  }
  draft.document.content.push({
    type: "paragraph",
    content: [{ type: "text", text: "用户：" }],
  });
  if (new TextEncoder().encode(JSON.stringify(draft)).length > 512_000)
    throw new RequestError("assistant_context_full");
  return draft;
}
export function assistantInitialDraft(
  initial?: PromptDraft,
  skill: AssistantSkill = "",
  execution: "discussion" | "image" = "discussion",
): PromptDraft {
  const draft = initial ? structuredClone(initial) : emptyPromptDraft();
  validatePromptDraft(draft);
  const selected = assistantSkills.find((item) => item.id === skill);
  if (selected)
    draft.document.content.unshift({
      type: "paragraph",
      content: [
        {
          type: "text",
          text:
            execution === "image"
              ? skill === "poster"
                ? "设计并生成一张完整海报，遵循用户指定的文案、主题、画幅与参考视觉。保持主体、品牌元素和文字要求一致。"
                : "生成商品图片，保持参考商品外观、材质、颜色与品牌标识一致，按用户的用途设计构图、背景和光线。生成数量使用用户确认的参数。"
              : `请围绕“${selected.title}”与我讨论创作方案。只对已提供的参考作出判断；需要实际生成图片或视频时，明确提出执行步骤，不声称已生成未执行的结果。`,
        },
      ],
    });
  return draft;
}

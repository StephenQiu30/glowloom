import type { GenerationSeed } from "@/components/generation/generation-dialog";
import { taskLabels, type Task } from "@/components/generation/contracts";
import { isProjectID, type Project } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import type { Node } from "./editor-state";

export const aiModes = [
  {
    id: "video-analysis",
    label: "视频反解",
    starter: "请根据视频参考或我提供的镜头描述，拆解镜头、节奏和画面提示词：",
    instruction:
      "逐镜拆解用户提供的视频内容。若没有可读取的视频或镜头描述，请明确说明并请求补充，不得猜测视频内容。",
  },
  {
    id: "storyboard",
    label: "创作分镜",
    starter: "请为以下创意编写分镜，包含景别、画面、动作和时长：",
    instruction:
      "根据创意输出可执行的文字分镜，列出镜号、景别、画面、动作、声音与时长。",
  },
  {
    id: "director",
    label: "全流程广告片导演",
    starter: "请为以下产品策划广告片，包含创意、脚本、分镜与制作计划：",
    instruction: "输出广告片的文字策划、脚本、分镜与制作计划。",
  },
  {
    id: "script",
    label: "剧本开发",
    starter: "请将以下故事创意发展为剧本，包含人物、冲突和场景：",
    instruction: "输出人物、冲突、故事结构及场景对白的剧本方案。",
  },
  {
    id: "short-film",
    label: "剧情短片",
    starter: "请为以下主题创作剧情短片方案，包含故事梗概、场景与镜头：",
    instruction: "输出剧情短片的梗概、场景、对白及文字分镜。",
  },
] as const;
export type AIMode = (typeof aiModes)[number]["id"];

function validTaskHead<T extends API.TaskSummary>(
  task: T | null,
): task is T & { id: string } {
  return (
    !!task &&
    isProjectID(task.id) &&
    Object.hasOwn(taskLabels, task.status ?? "") &&
    [task.capability, task.model_name, task.update_time].every(
      (value) => value === undefined || typeof value === "string",
    ) &&
    (task.failure_code == null || typeof task.failure_code === "string") &&
    (task.retryable == null || typeof task.retryable === "boolean") &&
    (task.cancel_requested === undefined ||
      typeof task.cancel_requested === "boolean")
  );
}

export function requireAIHistory(value: unknown, projectID: string) {
  const page = value as API.TaskPageResponse | null;
  if (
    !page ||
    !Array.isArray(page.items) ||
    page.items.length > 200 ||
    (page.next_cursor != null && typeof page.next_cursor !== "string")
  )
    throw new RequestError("invalid_receipt");
  const seen = new Set<string>();
  for (const item of page.items) {
    if (
      !validTaskHead(item) ||
      seen.has(item.id) ||
      item.project_id !== projectID ||
      !isProjectID(item.id)
    )
      throw new RequestError("invalid_receipt");
    seen.add(item.id);
  }
  return { items: page.items, next_cursor: page.next_cursor ?? undefined };
}

export function requireAITask(value: unknown, projectID: string): Task {
  const task = value as API.TaskDetail | null;
  if (
    !task ||
    !validTaskHead(task) ||
    task.project_id !== projectID ||
    (task.inputs !== undefined &&
      (!Array.isArray(task.inputs) ||
        task.inputs.some(
          (input) =>
            !input ||
            (input.role !== undefined && typeof input.role !== "string") ||
            (input.text !== undefined && typeof input.text !== "string"),
        ))) ||
    (task.outputs !== undefined &&
      (!Array.isArray(task.outputs) ||
        task.outputs.some(
          (output) =>
            !output ||
            !isProjectID(output.id) ||
            (output.sequence !== undefined &&
              (!Number.isInteger(output.sequence) || output.sequence < 0)) ||
            (output.text !== undefined && typeof output.text !== "string"),
        )))
  )
    throw new RequestError("invalid_receipt");
  return task as Task;
}

export function completedText(task: API.TaskDetail): string {
  if (task.capability !== "text.generate" || task.status !== "completed")
    return "";
  return (task.outputs ?? [])
    .filter(
      (output) =>
        output.kind === "json" &&
        output.moderation_status === "passed" &&
        typeof output.text === "string" &&
        output.text.trim(),
    )
    .toSorted((a, b) => (a.sequence ?? 0) - (b.sequence ?? 0))
    .map((output) => output.text)
    .join("\n\n");
}

export function selectedAIReference(node?: Node): GenerationSeed["reference"] {
  if (
    node?.ref_type !== "media_asset" ||
    !isProjectID(node.ref_id) ||
    !["image", "video", "audio"].includes(node.node_type)
  )
    return undefined;
  return {
    assetID: node.ref_id,
    kind: node.node_type as "image" | "video" | "audio",
  };
}

export function aiGenerationSeed(
  project: Project,
  mode: AIMode,
  prompt: string,
  selected?: Node,
  context?: Task,
): GenerationSeed {
  if (!prompt.trim()) throw new Error("请先输入创作需求。");
  if (selectedAIReference(selected))
    throw new Error(
      "当前文字生成不支持读取媒体，请移除本次参考并提供镜头或画面文字描述。",
    );
  const skill = aiModes.find((entry) => entry.id === mode);
  if (!skill) throw new Error("请选择创作模式。");
  const previous =
    context?.project_id === project.id ? completedText(context) : "";
  const selectedText =
    selected?.node_type === "text" ? selected.config.text : "";
  const compiled = [
    skill.instruction,
    "本次只输出文本方案。不得声称已生成媒体、读取无法访问的素材或执行了外部工具。",
    previous ? `上一轮已完成的方案：\n${previous}` : "",
    selectedText ? `选中的画布文本：\n${selectedText}` : "",
    `本次创作需求：\n${prompt.trim()}`,
  ]
    .filter(Boolean)
    .join("\n\n");
  if (Array.from(compiled).length > 10000)
    throw new Error(
      "需求与上下文合计超过 10000 字，请缩短内容或取消上一轮上下文。",
    );
  return {
    project,
    capability: "text",
    prompt: compiled,
  };
}

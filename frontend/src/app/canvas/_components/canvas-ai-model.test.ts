import { describe, expect, it } from "vitest";
import {
  aiGenerationSeed,
  completedText,
  requireAIHistory,
  requireAITask,
  selectedAIReference,
} from "./canvas-ai-model";
import type { Project } from "@/components/projects/project";
import type { Node } from "./editor-state";

const project: Project = {
  id: "11111111-1111-4111-8111-111111111111",
  canvas_id: null,
  name: "创作",
  aspect_ratio: "1:1",
  style_key: null,
  revision: 1,
  update_time: "2026-10-18T00:00:00Z",
};
const task = {
  id: "22222222-2222-4222-8222-222222222222",
  project_id: project.id,
  capability: "text.generate",
  status: "completed",
  inputs: [{ role: "prompt", text: "写一段开场" }],
  outputs: [
    {
      id: "33333333-3333-4333-8333-333333333333",
      kind: "json",
      moderation_status: "passed",
      text: "真实供应商文本",
    },
  ],
};

describe("画布 AI 数据边界", () => {
  it("接受真实接口的空分页与可空任务字段，保留非法值校验", () => {
    expect(
      requireAIHistory({ items: [], next_cursor: null }, project.id),
    ).toEqual({ items: [], next_cursor: undefined });
    expect(
      requireAIHistory(
        {
          items: [{ ...task, failure_code: null, retryable: null }],
          next_cursor: null,
        },
        project.id,
      ).items,
    ).toHaveLength(1);
    expect(() =>
      requireAIHistory({ items: [], next_cursor: 1 }, project.id),
    ).toThrow();
  });
  it("仅读取完成且审核通过的真实文本输出", () => {
    expect(completedText(requireAITask(task, project.id))).toBe(
      "真实供应商文本",
    );
    expect(
      completedText(
        requireAITask({ ...task, status: "submitted" }, project.id),
      ),
    ).toBe("");
    expect(
      completedText(
        requireAITask(
          {
            ...task,
            outputs: [{ ...task.outputs[0], moderation_status: "rejected" }],
          },
          project.id,
        ),
      ),
    ).toBe("");
    expect(
      completedText(requireAITask({ ...task, outputs: [] }, project.id)),
    ).toBe("");
  });
  it("拒绝跨项目、伪状态、重复任务和非法文本字段", () => {
    expect(() => requireAITask(task, "other")).toThrow();
    expect(() =>
      requireAIHistory(
        { items: [{ ...task, status: "invented" }] },
        project.id,
      ),
    ).toThrow();
    expect(() =>
      requireAIHistory({ items: [task, task] }, project.id),
    ).toThrow();
    expect(() =>
      requireAITask(
        { ...task, outputs: [{ ...task.outputs[0], text: 123 }] },
        project.id,
      ),
    ).toThrow();
  });
  it("五个模式交还真实文本报价，媒体参考在当前协议中明确阻止", () => {
    const media = {
      id: "node",
      title: "参考",
      node_type: "image",
      ref_type: "media_asset",
      ref_id: task.id,
    } as Node;
    expect(selectedAIReference(media)).toEqual({
      assetID: task.id,
      kind: "image",
    });
    expect(() => aiGenerationSeed(project, "script", "写开场", media)).toThrow(
      "当前文字生成不支持读取媒体",
    );
    expect(aiGenerationSeed(project, "storyboard", "森林")).toMatchObject({
      capability: "text",
    });
    expect(aiGenerationSeed(project, "video-analysis", "森林")).toMatchObject({
      capability: "text",
    });
  });
  it("连续创作仅加入显式选择的真实完成输出，拒绝空输入与超长上下文", () => {
    const seed = aiGenerationSeed(
      project,
      "script",
      "改得简短",
      undefined,
      requireAITask(task, project.id),
    );
    expect(seed.prompt).toContain("真实供应商文本");
    expect(seed.prompt).toContain("改得简短");
    expect(() => aiGenerationSeed(project, "script", " ")).toThrow();
    expect(() =>
      aiGenerationSeed(project, "script", "x".repeat(10001)),
    ).toThrow();
  });
});

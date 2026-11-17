import { describe, expect, it } from "vitest";
import {
  requireQuote,
  requireTask,
  fieldsForMode,
  initialParams,
  generationStatusLabel,
} from "@/components/generation/contracts";
const id = "11111111-1111-4111-8111-111111111111";
const project = "22222222-2222-4222-8222-222222222222";
describe("生成回执和参数边界", () => {
  it("接受可展示的过期报价，但拒绝不完整报价", () => {
    expect(
      requireQuote({
        operation_id: id,
        quote_micros: 0,
        available_micros: 10,
        confirmable: true,
        expires_at: "2000-01-01",
      }),
    ).toMatchObject({ expires_at: "2000-01-01" });
    expect(() =>
      requireQuote({
        operation_id: id,
        quote_micros: -1,
        expires_at: "2099-01-01",
      }),
    ).toThrow();
  });
  it("结算未定时区分审核拒绝和仍未知的执行结果", () => {
    const label = (failure_code: string) =>
      generationStatusLabel({ status: "manual", failure_code });
    expect(label("provider_input_content_rejected")).toBe("输入内容未通过审核");
    expect(label("provider_output_content_rejected")).toBe(
      "生成内容未通过审核",
    );
    expect(label("provider_request_rejected")).toBe("生成失败");
    expect(label("provider_transport_unknown")).toBe("暂未返回结果");
  });
  it("拒绝跨项目及未知状态任务", () => {
    expect(() =>
      requireTask({ id, project_id: id, status: "completed" }, project),
    ).toThrow();
    expect(() =>
      requireTask(
        { id, project_id: project, status: "finished" as API.OperationStatus },
        project,
      ),
    ).toThrow();
  });
  it("按模式筛选并仅使用已声明默认参数", () => {
    const f = fieldsForMode(
      [
        {
          field: "seed",
          label: "种子",
          type: "integer",
          component: "input",
          default: 0,
        },
        {
          field: "duration",
          label: "时长",
          type: "integer",
          component: "select",
          for_modes: ["text_to_video"],
          default: 5,
        },
      ],
      "text_to_image",
    );
    expect(initialParams(f)).toEqual({ seed: 0 });
    expect(() =>
      fieldsForMode([{ field: "__proto__", type: "string" }], "text_to_image"),
    ).toThrow();
  });
});

it("任务结果拒绝畸形媒体和跨任务回执", () => {
  expect(() =>
    requireTask(
      {
        id,
        project_id: project,
        status: "completed",
        outputs: [
          {
            id,
            kind: "image",
            media_asset_id: "bad",
            moderation_status: "passed",
          },
        ],
      },
      project,
    ),
  ).toThrow();
});

it("仅接受已完成文本任务的已审核有限文本输出", () => {
  const project = "00000000-0000-4000-8000-000000000001";
  const id = "00000000-0000-4000-8000-000000000002";
  const output = {
    id: "00000000-0000-4000-8000-000000000003",
    kind: "json",
    moderation_status: "passed",
    text: "真实结果",
  };
  const task: API.TaskDetail = {
    id,
    project_id: project,
    capability: "text.generate",
    status: "completed",
    outputs: [output],
  };
  expect(requireTask(task, project).outputs?.[0].text).toBe("真实结果");
  expect(() =>
    requireTask({ ...task, status: "submitted" }, project),
  ).toThrow();
  expect(() =>
    requireTask({ ...task, capability: "video.generate" }, project),
  ).toThrow();
  expect(() =>
    requireTask(
      { ...task, outputs: [{ ...output, text: "x".repeat(65537) }] },
      project,
    ),
  ).toThrow();
});

it.each(["text.structured", "document.generate"])(
  "文档能力%s沿用最终审核正文边界",
  (capability) => {
    const task: API.TaskDetail = {
      id,
      project_id: project,
      capability,
      status: "completed",
      outputs: [
        { id, kind: "json", moderation_status: "passed", text: "有限真实正文" },
      ],
    };
    expect(requireTask(task, project).outputs?.[0].text).toBe("有限真实正文");
    expect(() =>
      requireTask({ ...task, status: "succeeded" }, project),
    ).toThrow();
  },
);

it("keeps rebuilt queued, pending and received projections polling", async () => {
  const { activeTask, usableTextOutput } =
    await import("@/components/generation/contracts");
  for (const status of ["submitted", "succeeded", "ingesting"] as const) {
    const task = requireTask({ id, project_id: project, status }, project);
    expect(activeTask(task.status)).toBe(true);
  }
  const output = {
    id,
    kind: "json",
    text: "未经审核的生成文本",
    moderation_status: "skipped",
    output_policy: "unreviewed_text_v1",
  };
  expect(usableTextOutput(output)).toBe(true);
  expect(
    requireTask(
      {
        id,
        project_id: project,
        status: "completed",
        capability: "text.generate",
        outputs: [output],
      },
      project,
    ).outputs?.[0],
  ).toEqual(output);
  for (const bad of [
    { ...output, output_policy: undefined },
    { ...output, text: "光".repeat(22000) },
    { ...output, moderation_status: "pending" },
  ]) {
    expect(() =>
      requireTask(
        {
          id,
          project_id: project,
          status: "completed",
          capability: "text.generate",
          outputs: [bad],
        },
        project,
      ),
    ).toThrow();
  }
});

it("validates actual PostgreSQL Task projections when acceptance supplies them", async () => {
  const file = process.env.GLOWLOOM_TEST_AI_TASK_FIXTURE;
  if (!file) return;
  const { readFileSync } = await import("node:fs");
  const { activeTask, usableTextOutput } =
    await import("@/components/generation/contracts");
  const source = JSON.parse(readFileSync(file, "utf8"));
  const queued = requireTask(source.queued, source.project);
  const completed = requireTask(source.completed, source.project);
  expect(activeTask(queued.status)).toBe(true);
  expect(activeTask(completed.status)).toBe(false);
  expect(usableTextOutput(completed.outputs![0])).toBe(true);
});

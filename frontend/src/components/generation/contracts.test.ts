import { describe, expect, it } from "vitest";
import {
  requireQuote,
  requireTask,
  fieldsForMode,
  initialParams,
} from "./contracts";
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
  it("拒绝跨项目及未知状态任务", () => {
    expect(() =>
      requireTask({ id, project_id: id, status: "completed" }, project),
    ).toThrow();
    expect(() =>
      requireTask(
        { id, project_id: project, status: "finished" as CreationAPI.Status },
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

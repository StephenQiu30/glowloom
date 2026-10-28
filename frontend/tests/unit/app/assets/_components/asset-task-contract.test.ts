import { expect, it } from "vitest";
import { requireTask } from "@/components/generation/contracts";
const id = "11111111-1111-4111-8111-111111111111";
const project = "22222222-2222-4222-8222-222222222222";
const json = { id, kind: "json", moderation_status: "passed" };
const task = {
  id,
  project_id: project,
  status: "completed",
  capability: "text.structured",
  outputs: [json],
} satisfies API.TaskDetail;

it.each(["ingesting", "failed", "completed"] as const)(
  "formal document metadata is valid independently of the task capability during %s",
  (status) => {
    const output = {
      id,
      kind: "document",
      media_asset_id: id,
      moderation_status: "passed",
    };
    expect(
      requireTask(
        { ...task, status, capability: "image.generate", outputs: [output] },
        project,
      ).outputs?.[0],
    ).toEqual(output);
  },
);

it.each(["submitted", "succeeded", "ingesting", "completed"] as const)(
  "JSON candidate metadata needs no published body during %s",
  (status) => {
    expect(requireTask({ ...task, status }, project).outputs?.[0]).toEqual(
      json,
    );
    expect(
      requireTask({ ...task, status, capability: "video.generate" }, project)
        .outputs?.[0],
    ).toEqual(json);
  },
);

it("JSON metadata cannot smuggle a media ID, unreviewed text or nonfinal text", () => {
  expect(() =>
    requireTask(
      { ...task, outputs: [{ ...json, media_asset_id: id }] },
      project,
    ),
  ).toThrow();
  const invalidCandidates: API.TaskDetail[] = [
    { ...task, status: "succeeded", outputs: [{ ...json, text: "尚未完成" }] },
    {
      ...task,
      capability: "video.generate",
      outputs: [{ ...json, text: "非正文能力" }],
    },
    {
      ...task,
      outputs: [{ ...json, moderation_status: "pending", text: "未审核" }],
    },
    { ...task, outputs: [{ ...json, text: "x".repeat(65537) }] },
    { ...task, outputs: [{ ...json, text: "汉".repeat(21846) }] },
  ];
  for (const candidate of invalidCandidates)
    expect(() => requireTask(candidate, project)).toThrow();
  expect(
    requireTask(
      { ...task, outputs: [{ ...json, text: "最终审核通过的正文" }] },
      project,
    ).outputs?.[0].text,
  ).toBe("最终审核通过的正文");
});

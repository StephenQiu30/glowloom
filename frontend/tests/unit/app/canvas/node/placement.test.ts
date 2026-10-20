import { expect, it } from "vitest";
import { createResourceNode } from "@/app/canvas/document";
import { placeNode } from "@/app/canvas/node/placement";

it("批量导入避让尺寸不同的已有节点和同批次内容，忽略分组背景", () => {
  const existing = {
    ...createResourceNode("text", { x: 0, y: 0 }),
    width: 500,
    height: 600,
  };
  const candidate = {
    ...createResourceNode("text", { x: 400, y: 200 }),
    width: 288,
    height: 162,
  };
  const first = placeNode(candidate, [existing]);
  const second = placeNode(candidate, [existing, first]);
  expect(first.y + 162 + 24 <= 0 || first.y >= 624).toBe(true);
  expect(Math.abs(second.y - first.y)).toBeGreaterThanOrEqual(186);
  expect(placeNode(candidate, [{ ...existing, node_type: "group" }])).toEqual(
    candidate,
  );
});

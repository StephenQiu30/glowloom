import { expect, it } from "vitest";
import { connectionAnchor, groupWrapRect } from "./geometry";

it("分组外框为成员留出上标题和四周间距", () => {
  expect(
    groupWrapRect([
      { x: 100, y: 80, width: 40, height: 20 },
      { x: 200, y: 90, width: 40, height: 30 },
    ]),
  ).toEqual({ x: 76, y: 28, width: 188, height: 116 });
});

it("连线锚点分别落在节点左右中线", () => {
  const node = { x: 10, y: 20, width: 100, height: 40 };
  expect(connectionAnchor(node, "source")).toEqual({ x: 10, y: 40 });
  expect(connectionAnchor(node, "target")).toEqual({ x: 110, y: 40 });
});

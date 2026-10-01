import { describe, expect, it } from "vitest";
import { maskDraftCommands } from "./mask-draft";
import { CanvasNodeType } from "./model";
import { createNode } from "./document";
import type { MediaAsset } from "./queries";

const project = "10000000-0000-4000-8000-000000000000",
  original = "20000000-0000-4000-8000-000000000000";
const asset: MediaAsset = {
  id: "30000000-0000-4000-8000-000000000000",
  project_id: project,
  kind: "image",
  mime_type: "image/png",
  file_name: "mask.png",
  byte_size: 30,
  revision: 1,
  width: 300,
  height: 200,
};
describe("蒙版资产与生成草稿接管", () => {
  it("只保存正式素材引用和待确认用途，保留原图与蒙版连线", () => {
    const source = {
      ...createNode(CanvasNodeType.Image, { x: 0, y: 0 }),
      assetId: original,
    };
    const result = maskDraftCommands(
      source,
      asset,
      project,
      "替换手中杯子",
      1998,
    );
    const add = result.commands[0];
    if (add.type !== "AddNodes") throw new Error("缺少节点");
    const generation = add.nodes.find(
      (node) => node.type === CanvasNodeType.Generation,
    )!;
    expect(generation.generation?.inputs).toEqual([
      { role: "image", mediaAssetId: original },
      { role: "mask", mediaAssetId: asset.id },
    ]);
    expect(generation.generation?.capability).toBe("image.edit");
    expect(generation.generation?.modelProfileId).toBeNull();
    expect(result.commands[1]).toMatchObject({
      type: "Connect",
      edges: [
        { fromNodeId: source.id, toNodeId: generation.id },
        { toNodeId: generation.id },
      ],
    });
  });
  it("拒绝跨项目或非图片蒙版、空提示与超过节点容量", () => {
    const source = {
      ...createNode(CanvasNodeType.Image, { x: 0, y: 0 }),
      assetId: original,
    };
    expect(() =>
      maskDraftCommands(
        source,
        { ...asset, project_id: original },
        project,
        "修改",
        0,
      ),
    ).toThrow();
    expect(() =>
      maskDraftCommands(
        source,
        { ...asset, kind: "audio" },
        project,
        "修改",
        0,
      ),
    ).toThrow();
    expect(() => maskDraftCommands(source, asset, project, " ", 0)).toThrow();
    expect(() =>
      maskDraftCommands(source, asset, project, "修改", 1999),
    ).toThrow();
  });
});

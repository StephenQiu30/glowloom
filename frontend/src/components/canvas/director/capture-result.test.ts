import { describe, expect, it } from "vitest";
import { createNode } from "../document";
import { applyCommands } from "../document";
import { CanvasNodeType, type CanvasDocument } from "../model";
import type { MediaAsset } from "../queries";
import { createDirectorCaptureContext } from "./outputs";
import { directorCaptureResultCommands } from "./capture-result";

const projectId = "10000000-0000-4000-8000-000000000001";
const asset: MediaAsset = {
  id: "10000000-0000-4000-8000-000000000002",
  project_id: projectId,
  kind: "image",
  file_name: "shot.png",
  mime_type: "image/png",
  byte_size: 100,
  width: 320,
  height: 240,
  revision: 1,
};
function fixture() {
  const node = createNode(CanvasNodeType.Director, { x: 0, y: 0 });
  const document: CanvasDocument = {
    id: crypto.randomUUID(),
    projectId,
    name: "合成",
    revision: 1,
    scope: {},
    viewport: { x: 0, y: 0, k: 1 },
    nodes: [node],
    connections: [],
  };
  return {
    document,
    node,
    context: createDirectorCaptureContext(node.director!),
  };
}
describe("导演截图资源与图库原子回填", () => {
  it("保存图集、封面、真实图片节点及来源连线，未知结果回读后不重复创建", () => {
    const { document, node, context } = fixture();
    const commands = directorCaptureResultCommands(
      document,
      node.id,
      context,
      asset,
      1,
    );
    const saved = applyCommands(document, commands);
    expect(saved.nodes[0].director!.cover?.assetId).toBe(asset.id);
    expect(saved.nodes[0].director!.shots[0].screenshots).toHaveLength(1);
    expect(
      saved.nodes.filter((item) => item.assetId === asset.id),
    ).toHaveLength(1);
    expect(saved.connections).toHaveLength(1);
    expect(
      directorCaptureResultCommands(saved, node.id, context, asset, 1),
    ).toEqual([]);
  });
  it("已有图片资源只补图库及连线，拒绝跨项目、非图片及迟到场景", () => {
    const { document, node, context } = fixture();
    const existing = createNode(CanvasNodeType.Image, { x: 100, y: 100 });
    existing.assetId = asset.id;
    document.nodes.push(existing);
    const commands = directorCaptureResultCommands(
      document,
      node.id,
      context,
      asset,
      1,
    );
    expect(commands.some((command) => command.type === "AddNodes")).toBe(false);
    expect(commands.some((command) => command.type === "Connect")).toBe(true);
    expect(() =>
      directorCaptureResultCommands(
        document,
        node.id,
        context,
        {
          ...asset,
          project_id: crypto.randomUUID(),
        },
        1,
      ),
    ).toThrow();
    expect(() =>
      directorCaptureResultCommands(
        document,
        node.id,
        context,
        {
          ...asset,
          kind: "video",
        },
        1,
      ),
    ).toThrow();
    node.director!.background = "#123456";
    expect(() =>
      directorCaptureResultCommands(document, node.id, context, asset, 1),
    ).toThrow("场景已改变");
  });
});

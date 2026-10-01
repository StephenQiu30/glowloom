import { describe, expect, it } from "vitest";
import { applyCommands, createNode } from "./document";
import { exportResultCommands } from "./export-result";
import { mediaAssetsToCanvasNodes } from "./media-import";
import { CanvasNodeType, type CanvasDocument } from "./model";
import type { MediaAsset } from "./queries";

const asset: MediaAsset = {
  id: "30000000-0000-4000-8000-000000000000",
  project_id: "10000000-0000-4000-8000-000000000000",
  kind: "video",
  mime_type: "video/mp4",
  file_name: "output.mp4",
  byte_size: 100,
  width: 1920,
  height: 1080,
  duration_ms: 1500,
  revision: 2,
};
function document(): CanvasDocument {
  return {
    id: "20000000-0000-4000-8000-000000000000",
    projectId: asset.project_id,
    name: "合成画布",
    revision: 10,
    scope: {},
    viewport: { x: 0, y: 0, k: 1 },
    nodes: [createNode(CanvasNodeType.Timeline, { x: 0, y: 0 })],
    connections: [],
  };
}
describe("成片回填持久身份去重", () => {
  it("正式音频回填生成 audio 节点，同一输出重复回填不再次新增", () => {
    const current = document();
    const audio: MediaAsset = {
      ...asset,
      kind: "audio",
      mime_type: "audio/mp4",
      file_name: "output.m4a",
      width: undefined,
      height: undefined,
    };
    const saved = applyCommands(
      current,
      exportResultCommands(current, current.nodes[0].id, audio),
    );
    expect(saved.nodes.find((node) => node.assetId === audio.id)?.type).toBe(
      CanvasNodeType.Audio,
    );
    expect(exportResultCommands(saved, current.nodes[0].id, audio)).toEqual([]);
  });
  it("命令已执行但响应未知后读取实际文档，重复回填不再次新增节点或关系", () => {
    const initial = document(),
      source = initial.nodes[0].id;
    const commands = exportResultCommands(initial, source, asset);
    const saved = applyCommands(initial, commands);
    expect(
      saved.nodes.filter((node) => node.assetId === asset.id),
    ).toHaveLength(1);
    expect(saved.connections).toHaveLength(1);
    expect(exportResultCommands(saved, source, asset)).toEqual([]);
  });
  it("素材已在画布中时复用原节点，只补来源关系", () => {
    const current = document(),
      source = current.nodes[0].id;
    const existing = mediaAssetsToCanvasNodes([asset], { x: 500, y: 0 })[0];
    current.nodes.push(existing);
    const commands = exportResultCommands(current, source, asset);
    expect(commands).toEqual([
      {
        type: "Connect",
        edges: [
          {
            id: expect.any(String),
            fromNodeId: source,
            toNodeId: existing.id,
          },
        ],
      },
    ]);
    expect(applyCommands(current, commands).nodes).toHaveLength(2);
  });
  it("即使存在相同素材ID，也拒绝跨项目或已删除的时间线", () => {
    const current = document();
    expect(() =>
      exportResultCommands(current, current.nodes[0].id, {
        ...asset,
        project_id: current.id,
      }),
    ).toThrow();
    expect(() => exportResultCommands(current, current.id, asset)).toThrow();
  });
});

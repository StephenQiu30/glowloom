import { describe, expect, it } from "vitest";
import { applyCommands, createNode } from "../document";
import { CanvasNodeType, type CanvasDocument } from "../model";
import type { MediaAsset } from "../queries";
import { createDirectorCaptureContext } from "./outputs";
import { directorRecordingResultCommands } from "./recording-result";

const projectId = "10000000-0000-4000-8000-000000000001";
const asset: MediaAsset = {
  id: "10000000-0000-4000-8000-000000000002",
  project_id: projectId,
  kind: "video",
  file_name: "白膜.mp4",
  mime_type: "video/mp4",
  byte_size: 100,
  width: 320,
  height: 240,
  duration_ms: 1000,
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
describe("导演白膜正式资源回填", () => {
  it("只加入正式MP4和来源连线，未知结果恢复后复用身份", () => {
    const { document, node, context } = fixture();
    const commands = directorRecordingResultCommands(
      document,
      node.id,
      context,
      asset,
      1,
    );
    expect(commands.map((command) => command.type)).toEqual([
      "AddNodes",
      "Connect",
    ]);
    const saved = applyCommands(document, commands);
    expect(saved.nodes[0].director).toEqual(document.nodes[0].director);
    expect(
      directorRecordingResultCommands(saved, node.id, context, asset, 1),
    ).toEqual([]);
  });
  it("拒绝迟到场景、跨项目以及把WebM伪装成正式输出", () => {
    const { document, node, context } = fixture();
    expect(() =>
      directorRecordingResultCommands(
        document,
        node.id,
        context,
        { ...asset, mime_type: "video/webm" },
        1,
      ),
    ).toThrow();
    expect(() =>
      directorRecordingResultCommands(
        document,
        node.id,
        context,
        { ...asset, project_id: crypto.randomUUID() },
        1,
      ),
    ).toThrow();
    node.director!.objects[0].transform.position[0] += 1;
    expect(() =>
      directorRecordingResultCommands(document, node.id, context, asset, 1),
    ).toThrow("场景");
  });
});

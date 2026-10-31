import { describe, expect, it } from "vitest";
import {
  effectiveCinema,
  makeCinemaNode,
  updateCinemaSetting,
} from "@/components/canvas/cinema-kit/cinema-kit-graph";
import {
  cameraDefault,
  cinemaCatalog,
  type CinemaKit,
} from "@/components/canvas/cinema-kit/cinema-kit-model";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
import {
  connectCanvasDocumentSources,
  supportsCanvasConnection,
} from "@/components/canvas/lib/canvas-document-connections";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";

function fixture(nodes: CanvasNodeData[]): CanvasEditorState {
  return {
    nodes,
    connections: [],
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  };
}

function tool(kit: CinemaKit) {
  return makeCinemaNode(kit, { x: 0, y: 0 }, []);
}

function node(type: string) {
  return createCanvasNode(type, { x: 600, y: 0 });
}

const cameraKit: CinemaKit = { version: 1, camera: cameraDefault() };
const paletteKit: CinemaKit = {
  version: 1,
  palette: cinemaCatalog.palettes[0],
};

describe("advanced source combinations remain complete and atomic", () => {
  it("connects camera and palette to an image with both typed settings intact", () => {
    const camera = tool(cameraKit);
    const palette = tool(paletteKit);
    const target = node("image");
    const original = fixture([camera, palette, target]);
    const before = structuredClone(original);

    const result = connectCanvasDocumentSources(
      original,
      [camera.id, palette.id],
      target.id,
    );

    expect(original).toEqual(before);
    expect(result.nodes.map((item) => item.id)).toEqual(
      original.nodes.map((item) => item.id),
    );
    expect(result.connections.map((edge) => edge.documentData)).toMatchObject([
      {
        fromNodeId: camera.id,
        toNodeId: target.id,
        fromPortId: "camera",
        toPortId: "camera",
        purpose: "CINEMA_CAMERA",
        binding: { kind: "NODE_VALUE" },
      },
      {
        fromNodeId: palette.id,
        toNodeId: target.id,
        fromPortId: "palette",
        toPortId: "palette",
        purpose: "CINEMA_PALETTE",
        binding: { kind: "NODE_VALUE" },
      },
    ]);
    expect(effectiveCinema(result, target.id)).toEqual({
      version: 1,
      camera: cameraKit.camera,
      palette: paletteKit.palette,
    });
    expect(result.nodes.slice(0, 2)).toEqual(original.nodes.slice(0, 2));
  });

  it("rejects two distinct camera sources without retaining the first edge", () => {
    const first = tool(cameraKit);
    const second = tool(cameraKit);
    const target = node("image");
    const original = fixture([first, second, target]);
    const before = structuredClone(original);

    expect(() =>
      connectCanvasDocumentSources(original, [first.id, second.id], target.id),
    ).toThrow("这个输入已有连接");
    expect(original).toEqual(before);
    expect(original.connections).toEqual([]);
  });

  it("offers no common target for camera and motion and rejects either media target atomically", () => {
    const camera = tool(cameraKit);
    const motion = tool({ version: 1, motion: cinemaCatalog.motions[0] });
    const candidates = [
      "text",
      "image",
      "video",
      "audio",
      "timeline",
      "subject",
      "director",
    ];
    expect(
      candidates.filter((target) =>
        [camera, motion].every((source) =>
          supportsCanvasConnection(source.type, target, source),
        ),
      ),
    ).toEqual([]);

    for (const type of ["image", "video"]) {
      const target = node(type);
      const sources = type === "image" ? [camera, motion] : [motion, camera];
      const original = fixture([...sources, target]);
      const before = structuredClone(original);
      expect(() =>
        connectCanvasDocumentSources(
          original,
          sources.map((source) => source.id),
          target.id,
        ),
      ).toThrow("无法连接这些节点");
      expect(original).toEqual(before);
    }
  });

  it("rejects an embedded camera conflict after a valid palette without losing local settings", () => {
    const palette = tool(paletteKit);
    const camera = tool(cameraKit);
    const target = node("image");
    const original = updateCinemaSetting(
      fixture([palette, camera, target]),
      target.id,
      "camera",
      cameraKit,
    );
    const before = structuredClone(original);

    expect(() =>
      connectCanvasDocumentSources(
        original,
        [palette.id, camera.id],
        target.id,
      ),
    ).toThrow("目标已有同类设置");
    expect(original).toEqual(before);
    expect(original.connections).toEqual([]);
    expect(effectiveCinema(original, target.id)).toEqual(cameraKit);
  });

  it("retains both subject and image references in the complete video result", () => {
    // Structural authoring coverage only: an empty subject is not a claim that
    // resource readiness, execution preflight, or server persistence succeeded.
    const subject = node("subject");
    const image = node("image");
    const target = node("video");
    const original = fixture([subject, image, target]);
    const before = structuredClone(original);

    const result = connectCanvasDocumentSources(
      original,
      [subject.id, image.id],
      target.id,
    );

    expect(original).toEqual(before);
    expect(result.nodes.map((item) => item.id)).toEqual(
      original.nodes.map((item) => item.id),
    );
    expect(result.connections.map((edge) => edge.documentData)).toMatchObject([
      {
        fromNodeId: subject.id,
        toNodeId: target.id,
        fromPortId: "subject",
        toPortId: "reference_subjects",
        purpose: "REFERENCE_SUBJECT",
        binding: { kind: "NODE_VALUE" },
      },
      {
        fromNodeId: image.id,
        toNodeId: target.id,
        fromPortId: "image",
        toPortId: "reference_images",
        purpose: "REFERENCE_IMAGE",
        binding: { kind: "RUN_OUTPUT", selection: { mode: "ALL" } },
      },
    ]);
    expect(result.nodes[0].serviceData).toEqual(subject.serviceData);
    expect(result.nodes[2].documentData?.parameters?.mode).toBe("GENERATE");
  });

  it("rejects director with text when connecting to an existing image", () => {
    const text = node("text");
    const director = node("director");
    const target = node("image");
    const original = fixture([text, director, target]);
    const before = structuredClone(original);

    expect(() =>
      connectCanvasDocumentSources(original, [text.id, director.id], target.id),
    ).toThrow("无法连接这些节点");
    expect(original).toEqual(before);
    expect(original.nodes).toHaveLength(3);
    expect(original.connections).toEqual([]);
  });

  it.each(["timeline", "group"])(
    "rejects a mixed text + %s batch without leaving a valid subset",
    (type) => {
      const text = node("text");
      const unsupported = node(type);
      const target = node("image");
      const original = fixture([text, unsupported, target]);
      const before = structuredClone(original);

      expect(() =>
        connectCanvasDocumentSources(
          original,
          [text.id, unsupported.id],
          target.id,
        ),
      ).toThrow("无法连接这些节点");
      expect(original).toEqual(before);
      expect(original.connections).toEqual([]);
    },
  );
});

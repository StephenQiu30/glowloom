import { describe, it, expect } from "vitest";
import backendCatalog from "../../../../../backend/internal/prompt/domain/cinema_catalog.json";
import {
  cinemaCatalog,
  readCinemaKit,
  cameraDefault,
  cinemaSupports,
} from "@/components/canvas/cinema-kit/cinema-kit-model";
import {
  makeCinemaNode,
  effectiveCinema,
  updateCinemaSetting,
} from "@/components/canvas/cinema-kit/cinema-kit-graph";
import {
  sampledPalette,
  hexToHSV,
  hsvToHex,
  paletteHarmony,
} from "@/components/canvas/cinema-kit/cinema-palette";
import { connectCanvasDocument } from "@/components/canvas/lib/canvas-document-connections";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import { registerBuiltinNodes } from "@/components/canvas/nodes/builtin-nodes";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
registerBuiltinNodes();
function fixture(): CanvasEditorState {
  return {
    nodes: [
      makeCinemaNode(
        {
          version: 1,
          camera: {
            ...cameraDefault(),
            body: "现代数字",
            focal_length: "35mm",
          },
        },
        { x: 0, y: 0 },
        [],
      ),
      createCanvasNode("image", { x: 600, y: 0 }),
    ],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots",
    showImageInfo: false,
  };
}
describe("CinemaKit formal intent", () => {
  it("keeps source catalogs identical in browser and server", () => {
    expect(cinemaCatalog).toEqual(backendCatalog);
    expect(cinemaCatalog.palettes).toHaveLength(27);
    expect(cinemaCatalog.styles).toHaveLength(10);
    expect(cinemaCatalog.motions).toHaveLength(20);
  });
  it("rejects unknown enums, forged colors and provenance", () => {
    expect(() =>
      readCinemaKit({
        version: 1,
        camera: { ...cameraDefault(), body: "unknown" },
      }),
    ).toThrow();
    expect(() =>
      readCinemaKit({
        version: 1,
        palette: {
          ...cinemaCatalog.palettes[0],
          preset: cinemaCatalog.palettes[0].name,
          colors: ["#ABCDEF"],
        },
      }),
    ).toThrow();
    expect(() =>
      readCinemaKit({
        version: 1,
        style: cinemaCatalog.styles[0],
        sources: [
          {
            node_id: "11111111-1111-4111-8111-111111111111",
            kind: "style",
            hash: "-".repeat(64),
          },
        ],
      }),
    ).toThrow();
  });
  it("creates a real reference edge, retaining advanced config and effective typed input", () => {
    const g = fixture(),
      next = connectCanvasDocument(g, g.nodes[0].id, g.nodes[1].id);
    expect(next.connections).toHaveLength(1);
    expect(next.connections[0].documentData).toMatchObject({
      fromPortId: "camera",
      toPortId: "camera",
      purpose: "CINEMA_CAMERA",
      binding: { kind: "NODE_VALUE" },
    });
    expect(next.nodes[0].serviceData).toEqual(g.nodes[0].serviceData);
    expect(effectiveCinema(next, g.nodes[1].id)?.camera?.focal_length).toBe(
      "35mm",
    );
    expect(g.connections).toHaveLength(0);
  });
  it("replaces an incoming source with local intent atomically and clears only that kind", () => {
    const g = fixture(),
      linked = connectCanvasDocument(g, g.nodes[0].id, g.nodes[1].id),
      next = updateCinemaSetting(linked, g.nodes[1].id, "camera", {
        version: 1,
        camera: { ...cameraDefault(), focal_length: "85mm" },
      });
    expect(next.connections).toHaveLength(0);
    expect(effectiveCinema(next, g.nodes[1].id)?.camera?.focal_length).toBe(
      "85mm",
    );
    expect(() =>
      connectCanvasDocument(next, g.nodes[0].id, g.nodes[1].id),
    ).toThrow(/同类/);
    expect(
      effectiveCinema(
        updateCinemaSetting(next, g.nodes[1].id, "camera"),
        g.nodes[1].id,
      ),
    ).toBeUndefined();
  });
  it("enforces the source camera/image and motion/video boundary", () => {
    expect(cinemaSupports("camera", "video")).toBe(false);
    expect(cinemaSupports("motion", "image")).toBe(false);
    const g = fixture();
    g.nodes[1] = createCanvasNode("video", { x: 600, y: 0 });
    expect(() =>
      connectCanvasDocument(g, g.nodes[0].id, g.nodes[1].id),
    ).toThrow();
  });
  it("extracts actual opaque pixel colors and performs deterministic harmonies", () => {
    expect(
      sampledPalette(
        new Uint8ClampedArray([
          255, 0, 0, 255, 255, 0, 0, 255, 0, 0, 255, 255, 0, 255, 0, 0,
        ]),
      ),
    ).toEqual(["#FF0000", "#0000FF"]);
    expect(hsvToHex(120, 1, 1)).toBe("#00FF00");
    expect(hexToHSV("#FF0000")).toEqual({ h: 0, s: 1, v: 1 });
    expect(paletteHarmony("#FF0000", "互补色", 2)).toEqual([
      "#FF0000",
      "#00FFFF",
    ]);
  });
});

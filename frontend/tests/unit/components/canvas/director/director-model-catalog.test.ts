import fs from "node:fs/promises";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { Box3, Vector3 } from "three";
import { GLTFLoader } from "three/addons/loaders/GLTFLoader.js";
import {
  DIRECTOR_MODELS,
  DIRECTOR_MODEL_CATEGORIES,
  directorBuiltinModel,
} from "@/components/canvas/director/director-model-catalog";
import {
  DIRECTOR_ACTORS,
  DIRECTOR_GEOMETRIES,
} from "@/components/canvas/director/director-assets";
const publicPath = (url: string) => path.resolve("public", url.slice(1));
describe("bundled director asset evidence", () => {
  it("contains 32 actual CC0 models across eight populated categories", async () => {
    expect(DIRECTOR_MODELS).toHaveLength(32);
    expect(new Set(DIRECTOR_MODELS.map((m) => m.id)).size).toBe(32);
    for (const [category] of DIRECTOR_MODEL_CATEGORIES)
      expect(
        DIRECTOR_MODELS.filter((m) => m.category === category),
      ).toHaveLength(4);
    for (const item of DIRECTOR_MODELS) {
      expect(directorBuiltinModel(item.id)).toBe(item);
      expect(item.license).toBe("CC0-1.0");
      const bytes = await fs.readFile(publicPath(item.url));
      const gltf = await new GLTFLoader().parseAsync(
        Uint8Array.from(bytes).buffer,
        "",
      );
      const bounds = new Box3().setFromObject(gltf.scene, true),
        size = bounds.getSize(new Vector3());
      expect(size.y).toBeCloseTo(item.heightM, 4);
      expect(bounds.min.y).toBeCloseTo(0, 4);
      expect(bounds.getCenter(new Vector3()).x).toBeCloseTo(0, 4);
      expect(bounds.getCenter(new Vector3()).z).toBeCloseTo(0, 4);
      expect(
        (await fs.readFile(publicPath(item.thumbnail)))
          .subarray(0, 8)
          .toString("hex"),
      ).toBe("89504e470d0a1a0a");
    }
  });
  it("rejects arbitrary paths and URLs as builtin model ids", () => {
    for (const id of [
      "../model",
      "https://example.com/model.glb",
      "__proto__",
      "missing",
    ])
      expect(() => directorBuiltinModel(id)).toThrow("不存在");
  });
  it("ships distinct real thumbnails for 12 bodies and all 13 supported palette geometries", async () => {
    const images = await Promise.all(
      DIRECTOR_ACTORS.map((actor) => fs.readFile(publicPath(actor.thumbnail))),
    );
    expect(new Set(images.map((image) => image.toString("base64"))).size).toBe(
      12,
    );
    expect(DIRECTOR_GEOMETRIES.map(([id]) => id)).toEqual([
      "character",
      "box",
      "sphere",
      "cuboid",
      "cylinder",
      "cone",
      "pyramid",
      "torus",
      "plane",
      "capsule",
      "circle",
      "half_arc",
      "half_cylinder",
    ]);
    for (const [id] of DIRECTOR_GEOMETRIES)
      expect(
        (await fs.stat(`public/director/geometries/${id}.png`)).size,
      ).toBeGreaterThan(500);
  });
});

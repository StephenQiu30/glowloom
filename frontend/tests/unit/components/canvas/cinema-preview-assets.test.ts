import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import assets from "@/components/canvas/cinema-kit/preview-assets.json";
import { cinemaCatalog } from "@/components/canvas/cinema-kit/cinema-kit-model";

it("ships a complete, nonempty local preset library with matching content hashes", () => {
  for (const [kind, names] of [
    ["palette", cinemaCatalog.palettes.map((p) => p.name)],
    ["style", cinemaCatalog.styles],
    ["motion", cinemaCatalog.motions],
    ["camera", ["自动"]],
  ] as const) {
    for (const name of names) expect(assets).toHaveProperty(`${kind}:${name}`);
  }
  for (const asset of Object.values(assets)) {
    expect(asset.image).toMatch(/^\/cinema\/[a-f0-9]+\.(jpg|png|webp)$/);
    const bytes = readFileSync(resolve("public", asset.image.slice(1)));
    expect(bytes.length).toBeGreaterThan(0);
    expect(createHash("sha256").update(bytes).digest("hex")).toBe(asset.sha256);
  }
});

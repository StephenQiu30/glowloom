import { expect, it } from "vitest";
import {
  connectionFlowMesh,
  connectionFlowTexture,
  connectionFlowWindow,
} from "@/components/canvas/connections/connection-flow-model";

it("uses a shared 2600 ms phase and a length-dependent sweep with an exit gap", () => {
  const middle = connectionFlowWindow(400, 1300);
  expect(middle.progress).toBe(0.5);
  expect(middle.sweep).toBeCloseTo(220);
  expect(middle.head).toBeCloseTo(346);
  expect(middle.tail).toBeCloseTo(126);
  expect(connectionFlowWindow(400, 3900)).toEqual(
    connectionFlowWindow(400, 1300),
  );
  expect(connectionFlowWindow(100, 1300).sweep).toBe(120);
  expect(connectionFlowWindow(1000, 1300).sweep).toBe(312);
  expect(connectionFlowWindow(400, 2599).tail).toBeGreaterThan(400);
});
it("samples the curve in world units and changes geometry budget without changing the sweep", () => {
  const path = "M 0 0 C 100 0, 200 0, 300 0";
  const detailed = connectionFlowMesh(path);
  const fanout = connectionFlowMesh(path, true);
  expect(detailed.length).toBe(64 * 54);
  expect(fanout.length).toBe(16 * 54);
  expect(detailed[6]).toBe(300);
  expect(fanout[6]).toBe(300);
  expect(connectionFlowMesh("M bad").length).toBe(0);
});
it("keeps the entire core opaque, with blue endpoints and the warm internal crest", () => {
  const texture = connectionFlowTexture();
  expect([...texture.slice(0, 4)]).toEqual([0, 142, 229, 255]);
  expect([...texture.slice(-4)]).toEqual([0, 142, 229, 255]);
  const crest = Math.round(0.71 * 255) * 4;
  expect(texture[crest]).toBeGreaterThan(220);
  expect(texture[crest + 2]).toBeLessThan(35);
  for (let i = 3; i < texture.length; i += 4) expect(texture[i]).toBe(255);
});

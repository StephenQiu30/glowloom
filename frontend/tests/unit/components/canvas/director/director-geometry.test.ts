import { expect, it, vi } from "vitest";
import {
  Box3,
  BoxGeometry,
  Group,
  Mesh,
  MeshBasicMaterial,
  Texture,
  Vector3,
} from "three";
import {
  createDirectorPrimitive,
  disposeDirectorObject,
} from "@/components/canvas/director/director-scene";
import type { DirectorObject } from "@/components/canvas/director/director-model";
function primitive(name: string) {
  return createDirectorPrimitive({
    primitive: name,
    color: "#ffffff",
    kind: "primitive",
    cast_shadow: true,
    receive_shadow: true,
  } as DirectorObject);
}
it.each(["cuboid", "pyramid", "half_arc", "half_cylinder"])(
  "creates the real %s geometry with finite nonempty bounds",
  (name) => {
    const group = primitive(name),
      bounds = new Box3().setFromObject(group),
      size = bounds.getSize(new Vector3());
    expect(
      size.toArray().every((value) => Number.isFinite(value) && value > 0),
    ).toBe(true);
    expect(group.children[0]).toBeInstanceOf(Mesh);
    if (name === "cuboid") expect(size.toArray()).toEqual([1.5, 1, 0.75]);
    if (name === "pyramid") expect(size.toArray()).toEqual([1, 1, 1]);
    if (name === "half_arc") {
      expect(size.y).toBeLessThan(size.x);
      expect(size.z).toBeCloseTo(0.3);
    }
    if (name === "half_cylinder") {
      expect(size.x).toBeCloseTo(1);
      expect(size.y).toBeCloseTo(1);
      expect(size.z).toBeCloseTo(0.5);
    }
    disposeDirectorObject(group);
  },
);
it("keeps legacy polyhedron scenes readable", () => {
  const group = primitive("icosahedron");
  expect((group.children[0] as Mesh).geometry.type).toBe("IcosahedronGeometry");
  disposeDirectorObject(group);
});

it("releases shared geometry, material and texture once per disposed scene", () => {
  const texture = new Texture(),
    material = new MeshBasicMaterial({ map: texture }),
    geometry = new BoxGeometry(),
    group = new Group();
  group.add(new Mesh(geometry, material), new Mesh(geometry, material));
  const textureDispose = vi.spyOn(texture, "dispose"),
    materialDispose = vi.spyOn(material, "dispose"),
    geometryDispose = vi.spyOn(geometry, "dispose");
  disposeDirectorObject(group);
  expect(textureDispose).toHaveBeenCalledTimes(1);
  expect(materialDispose).toHaveBeenCalledTimes(1);
  expect(geometryDispose).toHaveBeenCalledTimes(1);
});

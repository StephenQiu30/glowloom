import { expect, it } from "vitest";
import { Box3, BoxGeometry, Group, Mesh, MeshBasicMaterial } from "three";
import { directorGroundPosition } from "@/components/canvas/director/director-ground";
it("places rotated scaled mesh bounds on the ground without changing its horizontal location", () => {
  const stage = new Group(),
    target = new Mesh(new BoxGeometry(), new MeshBasicMaterial());
  stage.rotation.set(0.2, 0.5, 0.1);
  stage.position.set(2, 0.4, -1);
  stage.add(target);
  target.position.set(3, 7, -2);
  target.scale.set(2, 3, 1);
  target.rotation.z = 0.5;
  stage.updateMatrixWorld(true);
  const before = new Box3().setFromObject(target, true);
  target.position.fromArray(directorGroundPosition(target, 0.75));
  const after = new Box3().setFromObject(target, true);
  expect(after.min.y).toBeCloseTo(0.75);
  expect(after.min.x).toBeCloseTo(before.min.x);
  expect(after.min.z).toBeCloseTo(before.min.z);
  expect(directorGroundPosition(target, 0.75)).toEqual(
    expect.arrayContaining(
      target.position.toArray().map((value) => expect.closeTo(value, 6)),
    ),
  );
  target.geometry.dispose();
  target.material.dispose();
});
it("uses the object origin for cameras and other objects without mesh bounds", () => {
  const target = new Group();
  target.position.set(1, 5, 3);
  expect(directorGroundPosition(target, 0.25)).toEqual([1, 0.25, 3]);
});

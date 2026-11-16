import { Box3, Object3D, Vector3 } from "three";

/** Move the rendered bounds onto the world ground, preserving parent coordinates. */
export function directorGroundPosition(
  target: Object3D,
  height: number,
): number[] {
  target.updateWorldMatrix(true, true);
  const bounds = new Box3().setFromObject(target, true);
  const position = target.getWorldPosition(new Vector3());
  position.y += height - (bounds.isEmpty() ? position.y : bounds.min.y);
  return target.parent
    ? target.parent.worldToLocal(position).toArray()
    : position.toArray();
}

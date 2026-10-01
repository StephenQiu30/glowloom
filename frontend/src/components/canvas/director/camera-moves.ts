// Adapted from BeefTV 1ae25027 canvas-director-workbench.tsx cameraMoveTransform; MIT.
import type { DirectorTransform, DirectorShot, DirectorVec3 } from "./model";
export function cameraMoveTransform(
  transform: DirectorTransform,
  move: DirectorShot["cameraMove"],
): DirectorTransform {
  const [x, y, z] = transform.position;
  const offsets: Record<DirectorShot["cameraMove"], DirectorVec3> = {
    static: [0, 0, 0],
    push_in: [0, 0, -2],
    pull_out: [0, 0, 2],
    pan_left: [-2, 0, 0],
    pan_right: [2, 0, 0],
    tilt_up: [0, 1.5, 0],
    tilt_down: [0, -1.2, 0],
    orbit_left: [-2.5, 0, -1.5],
    orbit_right: [2.5, 0, -1.5],
    handheld: [0.18, 0.08, -0.15],
  };
  const offset = offsets[move];
  return {
    ...transform,
    position: [x + offset[0], y + offset[1], z + offset[2]],
  };
}

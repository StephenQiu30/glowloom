import { Euler, Quaternion, Vector3 } from "three";
/** Light direction points from its position toward the scene; angles use degrees. */
export function directorLightAngles(rotation: number[]) {
  const direction = new Vector3(0, 0, -1).applyEuler(
    new Euler(rotation[0], rotation[1], rotation[2]),
  );
  return {
    horizontal: (Math.atan2(direction.x, -direction.z) * 180) / Math.PI,
    elevation:
      (Math.asin(Math.max(-1, Math.min(1, -direction.y))) * 180) / Math.PI,
  };
}
export function directorLightRotation(
  horizontal: number,
  elevation: number,
): number[] {
  const h = (horizontal * Math.PI) / 180,
    e = (Math.max(-90, Math.min(90, elevation)) * Math.PI) / 180;
  const direction = new Vector3(
    Math.sin(h) * Math.cos(e),
    -Math.sin(e),
    -Math.cos(h) * Math.cos(e),
  );
  const rotation = new Euler().setFromQuaternion(
    new Quaternion().setFromUnitVectors(new Vector3(0, 0, -1), direction),
  );
  return [rotation.x, rotation.y, rotation.z];
}

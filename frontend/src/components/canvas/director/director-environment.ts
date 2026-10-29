import { Euler, Quaternion, Vector3 } from "three";
import {
  readDirector,
  directorTransform,
  type Director,
} from "./director-model";
export const DIRECTOR_ENVIRONMENTS = [
  ["studio", "影棚"],
  ["day", "日光"],
  ["night", "夜景"],
] as const;
export function applyDirectorEnvironment(
  scene: Director,
  preset: string,
): Director {
  const next = readDirector(scene);
  const night = preset === "night",
    day = preset === "day";
  next.background = night ? "#101827" : day ? "#b8cbe0" : "#171717";
  next.environment_intensity = night ? 0.3 : 1;
  next.exposure = 1.05;
  next.lights = [
    {
      name: "主光",
      position: [4, 6, 4],
      intensity: night ? 2 : 3,
      color: night ? "#8badff" : "#fff4df",
    },
    {
      name: "补光",
      position: [-4, 3, 2],
      intensity: night ? 0.4 : 1,
      color: "#d9e6ff",
    },
    {
      name: "轮廓光",
      position: [0, 5, -4],
      intensity: night ? 3 : 2,
      color: "#ffffff",
    },
  ].map((light) => {
    const direction = new Vector3(0, 1, 0)
      .sub(new Vector3().fromArray(light.position))
      .normalize();
    const rotation = new Euler().setFromQuaternion(
      new Quaternion().setFromUnitVectors(new Vector3(0, 0, -1), direction),
    );
    return {
      id: crypto.randomUUID(),
      name: light.name,
      type: "directional",
      color: light.color,
      intensity: light.intensity,
      transform: {
        ...directorTransform(light.position),
        rotation: [rotation.x, rotation.y, rotation.z],
      },
      cast_shadow: light.name === "主光",
      locked: false,
    };
  });
  return readDirector(next);
}

// Adapted MIT implementation; source attribution: notices/third-party-MIT.txt.
import { Bone, Matrix4, SkinnedMesh, Vector3, type Object3D } from "three";
import {
  directorBodyProfile,
  type DirectorBodyProfile,
} from "./director-body-profile";
type DirectorActorPresetId = string;

const HIP_Y = 0.95;
const NECK_Y = 1.56;

function blend(a: number, b: number, t: number): number {
  return a + (b - a) * Math.min(1, Math.max(0, t));
}

function widthAt(y: number, profile: DirectorBodyProfile): number {
  if (y < HIP_Y) return profile.hips;
  if (y < 1.13) return blend(profile.hips, profile.waist, (y - HIP_Y) / 0.18);
  if (y < 1.43)
    return blend(profile.waist, profile.shoulders, (y - 1.13) / 0.3);
  return blend(profile.shoulders, profile.headWidth, (y - 1.43) / 0.21);
}

function depthAt(y: number, profile: DirectorBodyProfile): number {
  if (y < 0.82) return profile.hips;
  if (y < 1.08)
    return blend(profile.hips, profile.bellyDepth, (y - 0.82) / 0.26);
  if (y < 1.3)
    return blend(profile.bellyDepth, profile.chestDepth, (y - 1.08) / 0.22);
  return blend(profile.chestDepth, profile.headWidth, (y - 1.3) / 0.34);
}

function deform(point: Vector3, profile: DirectorBodyProfile): Vector3 {
  const y = point.y;
  const sideLeg = point.x >= 0 ? profile.leftLeg : profile.rightLeg;
  const leg = blend(profile.leg, sideLeg, Math.abs(point.x) / 0.08);
  const height =
    y < HIP_Y
      ? HIP_Y * profile.leg + (y - HIP_Y) * leg
      : y < NECK_Y
        ? HIP_Y * profile.leg + (y - HIP_Y) * profile.torso
        : HIP_Y * profile.leg +
          (NECK_Y - HIP_Y) * profile.torso +
          (y - NECK_Y) * profile.headHeight;
  const width = widthAt(y, profile);
  const armBlend = Math.min(1, Math.max(0, (Math.abs(point.x) - 0.28) / 0.17));
  const horizontal = blend(
    width,
    point.x >= 0 ? profile.leftArm : profile.rightArm,
    armBlend,
  );
  const depth = depthAt(y, profile);
  return point.set(point.x * horizontal, height, point.z * depth);
}

/** Reshape a private clone of the CC0 body and its rest skeleton together. Never mutate the shared GLB source. */
export function reshapeDirectorQuaterniusActor(
  root: Object3D,
  preset: DirectorActorPresetId,
  overrides?: Partial<DirectorBodyProfile>,
): void {
  const profile = directorBodyProfile(preset, overrides);
  if (Object.values(profile).every((value) => value === 1)) return;
  root.updateMatrixWorld(true);
  const rootToWorld = root.matrixWorld.clone();
  const worldToRoot = rootToWorld.clone().invert();
  const bones: Bone[] = [];
  const originalBonePositions = new Map<Bone, Vector3>();
  const meshes: SkinnedMesh[] = [];
  root.traverse((object) => {
    if (object instanceof Bone) {
      bones.push(object);
      originalBonePositions.set(
        object,
        object.getWorldPosition(new Vector3()).applyMatrix4(worldToRoot),
      );
    }
    if (object instanceof SkinnedMesh) meshes.push(object);
  });

  for (const mesh of meshes) {
    const geometry = mesh.geometry.clone();
    const positions = geometry.getAttribute("position");
    const meshToWorld = mesh.matrixWorld.clone();
    const worldToMesh = meshToWorld.clone().invert();
    const point = new Vector3();
    for (let index = 0; index < positions.count; index += 1) {
      point
        .fromBufferAttribute(positions, index)
        .applyMatrix4(meshToWorld)
        .applyMatrix4(worldToRoot);
      deform(point, profile)
        .applyMatrix4(rootToWorld)
        .applyMatrix4(worldToMesh);
      positions.setXYZ(index, point.x, point.y, point.z);
    }
    positions.needsUpdate = true;
    geometry.computeVertexNormals();
    geometry.computeBoundingBox();
    geometry.computeBoundingSphere();
    mesh.geometry.dispose();
    mesh.geometry = geometry;
  }

  for (const bone of bones) {
    const original = originalBonePositions.get(bone)!;
    const parentInverse =
      bone.parent?.matrixWorld.clone().invert() ?? new Matrix4();
    bone.position.copy(
      deform(original.clone(), profile)
        .applyMatrix4(rootToWorld)
        .applyMatrix4(parentInverse),
    );
    bone.updateMatrixWorld(true);
  }
  root.updateMatrixWorld(true);
  for (const skeleton of new Set(meshes.map((mesh) => mesh.skeleton)))
    skeleton.calculateInverses();
}

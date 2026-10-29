// Adapted from BeefTV 2327ee2. MIT: notices/BeefTV-MIT.txt.
import { Euler, Quaternion } from "three";
export function directorPoseBoneDeltas(pose: string): Record<string, number[]> {
  // Soldier 的左右上臂局部 Z 轴方向一致，正向旋转才会把两侧手臂从 T Pose 放下。
  const armsDown = {
    leftUpperArm: poseQuaternion(0, 0, 1.28),
    rightUpperArm: poseQuaternion(0, 0, 1.28),
  };
  const poses: Record<string, Record<string, number[]>> = {
    neutral: armsDown,
    stand: armsDown,
    t_pose: {},
    walk: {
      ...armsDown,
      leftUpperArm: poseQuaternion(0.36, 0, 1.2),
      rightUpperArm: poseQuaternion(-0.36, 0, 1.2),
      leftUpperLeg: poseQuaternion(-0.32, 0, 0),
      rightUpperLeg: poseQuaternion(0.32, 0, 0),
    },
    run: {
      ...armsDown,
      leftUpperArm: poseQuaternion(0.75, 0, 1.05),
      rightUpperArm: poseQuaternion(-0.75, 0, 1.05),
      leftLowerArm: poseQuaternion(-0.7, 0, 0),
      rightLowerArm: poseQuaternion(-0.7, 0, 0),
      leftUpperLeg: poseQuaternion(-0.65, 0, 0),
      rightUpperLeg: poseQuaternion(0.55, 0, 0),
      rightLowerLeg: poseQuaternion(0.8, 0, 0),
    },
    sit: {
      ...armsDown,
      leftUpperLeg: poseQuaternion(-1.35, 0, 0),
      rightUpperLeg: poseQuaternion(-1.35, 0, 0),
      leftLowerLeg: poseQuaternion(1.25, 0, 0),
      rightLowerLeg: poseQuaternion(1.25, 0, 0),
    },
    squat: {
      ...armsDown,
      hips: poseQuaternion(0.25, 0, 0),
      leftUpperLeg: poseQuaternion(-0.75, 0, 0),
      rightUpperLeg: poseQuaternion(-0.75, 0, 0),
      leftLowerLeg: poseQuaternion(1.2, 0, 0),
      rightLowerLeg: poseQuaternion(1.2, 0, 0),
    },
    kneel_single: {
      ...armsDown,
      leftUpperLeg: poseQuaternion(-0.95, 0, 0),
      leftLowerLeg: poseQuaternion(1.45, 0, 0),
      rightUpperLeg: poseQuaternion(-0.35, 0, 0),
      rightLowerLeg: poseQuaternion(0.75, 0, 0),
    },
    kneel_double: {
      ...armsDown,
      leftUpperLeg: poseQuaternion(-0.55, 0, 0),
      rightUpperLeg: poseQuaternion(-0.55, 0, 0),
      leftLowerLeg: poseQuaternion(1.45, 0, 0),
      rightLowerLeg: poseQuaternion(1.45, 0, 0),
    },
    hands_hips: {
      leftUpperArm: poseQuaternion(0, 0, 0.75),
      rightUpperArm: poseQuaternion(0, 0, 0.75),
      leftLowerArm: poseQuaternion(-0.1, 0.2, -1.5),
      rightLowerArm: poseQuaternion(-0.1, -0.2, 1.5),
    },
    lean: {
      ...armsDown,
      hips: poseQuaternion(0, 0, 0.18),
      spine: poseQuaternion(0, 0, -0.12),
      head: poseQuaternion(0, 0, -0.08),
    },
    bow: {
      ...armsDown,
      hips: poseQuaternion(0.5, 0, 0),
      spine: poseQuaternion(0.28, 0, 0),
      head: poseQuaternion(-0.18, 0, 0),
    },
    think: {
      ...armsDown,
      rightUpperArm: poseQuaternion(-0.25, 0, 0.55),
      rightLowerArm: poseQuaternion(-1.35, 0, 0.3),
      head: poseQuaternion(0.05, -0.22, 0),
    },
    fight: {
      leftUpperArm: poseQuaternion(-0.65, 0, 0.7),
      rightUpperArm: poseQuaternion(-0.55, 0, 0.65),
      leftLowerArm: poseQuaternion(-1.2, 0, 0),
      rightLowerArm: poseQuaternion(-1.25, 0, 0),
      chest: poseQuaternion(0, 0.2, 0),
      leftUpperLeg: poseQuaternion(-0.15, 0, 0),
      rightUpperLeg: poseQuaternion(0.2, 0, 0),
    },
    kick: {
      ...armsDown,
      leftUpperArm: poseQuaternion(0.3, 0, 1.1),
      rightUpperArm: poseQuaternion(-0.3, 0, 1.1),
      rightUpperLeg: poseQuaternion(-1.1, 0, 0),
      rightLowerLeg: poseQuaternion(0.35, 0, 0),
    },
    throw: {
      leftUpperArm: poseQuaternion(-0.35, 0.2, 0.35),
      rightUpperArm: poseQuaternion(-1.2, 0, 0.25),
      rightLowerArm: poseQuaternion(-1.05, 0, 0),
      chest: poseQuaternion(0, -0.3, 0),
    },
    push: {
      leftUpperArm: poseQuaternion(-0.9, 0, 0.3),
      rightUpperArm: poseQuaternion(-0.9, 0, 0.3),
      leftLowerArm: poseQuaternion(-0.35, 0, 0),
      rightLowerArm: poseQuaternion(-0.35, 0, 0),
      chest: poseQuaternion(0.15, 0, 0),
    },
    wave: {
      ...armsDown,
      rightUpperArm: poseQuaternion(0, 0, -0.35),
      rightLowerArm: poseQuaternion(0, 0, -1.45),
      rightHand: poseQuaternion(0, 0, -0.3),
    },
    reach: {
      ...armsDown,
      rightUpperArm: poseQuaternion(-1.35, 0, -0.05),
      rightLowerArm: poseQuaternion(-0.1, 0, 0),
    },
    arms_crossed: {
      leftUpperArm: poseQuaternion(-0.65, 0, 0.65),
      rightUpperArm: poseQuaternion(-0.65, 0, 0.65),
      leftLowerArm: poseQuaternion(-1.2, 0.15, -0.4),
      rightLowerArm: poseQuaternion(-1.2, -0.15, 0.4),
    },
    phone: {
      ...armsDown,
      leftUpperArm: poseQuaternion(-0.45, 0, 0.95),
      rightUpperArm: poseQuaternion(-0.45, 0, 0.95),
      leftLowerArm: poseQuaternion(-1.1, 0, 0.15),
      rightLowerArm: poseQuaternion(-1.1, 0, -0.15),
      head: poseQuaternion(0.28, 0, 0),
    },
  };
  return poses[pose] ?? poses.stand;
}

function poseQuaternion(x: number, y: number, z: number): number[] {
  return new Quaternion().setFromEuler(new Euler(x, y, z)).toArray();
}

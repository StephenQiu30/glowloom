// Adapted from BeefTV 2327ee2. MIT: notices/BeefTV-MIT.txt.
export type DirectorBodyProfile = {
  leg: number;
  torso: number;
  headHeight: number;
  shoulders: number;
  waist: number;
  hips: number;
  arms: number;
  headWidth: number;
  chestDepth: number;
  bellyDepth: number;
  leftArm: number;
  rightArm: number;
  leftLeg: number;
  rightLeg: number;
};

// Upstream body profiles; this adaptation shares each profile across both bundled base meshes.
// The Quaternius body is 1.81 units tall in its rest pose, with hips at .95 and neck at 1.56.
const BODY_PROFILES: Partial<
  Record<
    string,
    Omit<DirectorBodyProfile, "leftArm" | "rightArm" | "leftLeg" | "rightLeg">
  >
> = {
  athletic: {
    leg: 1.08,
    torso: 1.11,
    headHeight: 1.03,
    shoulders: 1.18,
    waist: 1.04,
    hips: 1.08,
    arms: 1.09,
    headWidth: 1.01,
    chestDepth: 1.16,
    bellyDepth: 1.03,
  },
  slim: {
    leg: 0.95,
    torso: 0.94,
    headHeight: 0.97,
    shoulders: 0.84,
    waist: 0.76,
    hips: 0.84,
    arms: 0.9,
    headWidth: 0.96,
    chestDepth: 0.83,
    bellyDepth: 0.78,
  },
  teen: {
    leg: 0.8,
    torso: 0.84,
    headHeight: 0.96,
    shoulders: 0.86,
    waist: 0.84,
    hips: 0.88,
    arms: 0.83,
    headWidth: 1.08,
    chestDepth: 0.88,
    bellyDepth: 0.86,
  },
  child: {
    leg: 0.55,
    torso: 0.62,
    headHeight: 1.0,
    shoulders: 0.77,
    waist: 0.83,
    hips: 0.86,
    arms: 0.66,
    headWidth: 1.27,
    chestDepth: 0.82,
    bellyDepth: 0.88,
  },
  broad: {
    leg: 0.96,
    torso: 1.0,
    headHeight: 0.96,
    shoulders: 1.16,
    waist: 1.2,
    hips: 1.19,
    arms: 1.06,
    headWidth: 1.04,
    chestDepth: 1.16,
    bellyDepth: 1.29,
  },
  chibi: {
    leg: 0.25,
    torso: 0.43,
    headHeight: 1.2,
    shoulders: 0.76,
    waist: 0.92,
    hips: 1.0,
    arms: 0.6,
    headWidth: 1.58,
    chestDepth: 0.92,
    bellyDepth: 1.0,
  },
};

export const DIRECTOR_BODY_FIELDS = [
  ["leg", "腿长"],
  ["torso", "躯干长度"],
  ["headHeight", "头部高度"],
  ["shoulders", "肩宽"],
  ["waist", "腰围"],
  ["hips", "胯宽"],
  ["arms", "手臂比例"],
  ["headWidth", "头部宽度"],
  ["chestDepth", "胸部厚度"],
  ["bellyDepth", "腹部厚度"],
  ["leftArm", "左臂比例"],
  ["rightArm", "右臂比例"],
  ["leftLeg", "左腿长度"],
  ["rightLeg", "右腿长度"],
] as const;
export const DIRECTOR_BODY_MIN = 0.25;
export const DIRECTOR_BODY_MAX = 1.6;
export function supportsDirectorBody(object: {
  kind: string;
  builtin_actor?: string;
  asset_id?: string;
}): boolean {
  return (
    object.kind === "actor" &&
    !object.asset_id &&
    /^(male|female)_(standard|teen|child|slim|athletic|broad)$/.test(
      object.builtin_actor ?? "",
    )
  );
}
export function validateDirectorBodyProfile(
  profile: unknown,
): asserts profile is Partial<DirectorBodyProfile> {
  if (!profile || typeof profile !== "object" || Array.isArray(profile))
    throw new Error("身体参数无效");
  for (const [key, value] of Object.entries(profile)) {
    if (
      !DIRECTOR_BODY_FIELDS.some(([field]) => field === key) ||
      typeof value !== "number" ||
      !Number.isFinite(value) ||
      value < DIRECTOR_BODY_MIN ||
      value > DIRECTOR_BODY_MAX
    )
      throw new Error("身体参数无效");
  }
}
export function directorBodyProfile(
  preset: string,
  overrides?: Partial<DirectorBodyProfile>,
): DirectorBodyProfile {
  const name = preset.replace(/^(male|female)_/, "");
  if (name !== "standard" && !BODY_PROFILES[name])
    throw new Error("角色体型预设无效");
  if (overrides !== undefined) validateDirectorBodyProfile(overrides);
  const profile = {
    leg: 1,
    torso: 1,
    headHeight: 1,
    shoulders: 1,
    waist: 1,
    hips: 1,
    arms: 1,
    headWidth: 1,
    chestDepth: 1,
    bellyDepth: 1,
    ...BODY_PROFILES[name],
    ...overrides,
  };
  return {
    ...profile,
    leftArm: overrides?.leftArm ?? profile.arms,
    rightArm: overrides?.rightArm ?? profile.arms,
    leftLeg: overrides?.leftLeg ?? profile.leg,
    rightLeg: overrides?.rightLeg ?? profile.leg,
  };
}

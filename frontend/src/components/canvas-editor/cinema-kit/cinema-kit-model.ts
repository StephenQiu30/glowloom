import catalog from "./catalog.json";
export { catalog as cinemaCatalog };
export const cinemaKinds = ["camera", "palette", "style", "motion"] as const;
export type CinemaKind = (typeof cinemaKinds)[number];
export const cinemaLabels: Record<CinemaKind, string> = {
  camera: "相机",
  palette: "配色",
  style: "风格",
  motion: "运镜",
};
export type CinemaCamera = {
  body: string;
  lens: string;
  focal_length: string;
  aperture: string;
};
export type CinemaPalette = { preset?: string; name: string; colors: string[] };
export type CinemaKit = {
  version: 1;
  camera?: CinemaCamera;
  palette?: CinemaPalette;
  style?: string;
  motion?: string;
  sources?: { node_id: string; kind: CinemaKind; hash: string }[];
};
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);
const allowed = (v: Record<string, unknown>, keys: string[]) =>
  Object.keys(v).every((k) => keys.includes(k));
export function readCinemaKit(value: unknown): CinemaKit {
  if (
    !object(value) ||
    !allowed(value, ["version", ...cinemaKinds, "sources"]) ||
    value.version !== 1 ||
    !cinemaKinds.some((k) => value[k] !== undefined)
  )
    throw new Error("导演工具箱设置格式无效");
  if (value.camera !== undefined) {
    const c = value.camera;
    if (
      !object(c) ||
      !allowed(c, ["body", "lens", "focal_length", "aperture"]) ||
      !catalog.camera.includes(String(c.body)) ||
      !catalog.lens.includes(String(c.lens)) ||
      !catalog.focalLength.includes(String(c.focal_length)) ||
      !catalog.aperture.includes(String(c.aperture))
    )
      throw new Error("相机设置已失效");
  }
  if (
    value.style !== undefined &&
    !catalog.styles.includes(String(value.style))
  )
    throw new Error("风格设置已失效");
  if (
    value.motion !== undefined &&
    !catalog.motions.includes(String(value.motion))
  )
    throw new Error("运镜设置已失效");
  if (value.palette !== undefined) {
    const p = value.palette;
    if (
      !object(p) ||
      !allowed(p, ["preset", "name", "colors"]) ||
      typeof p.name !== "string" ||
      !p.name.trim() ||
      p.name.length > 128 ||
      !Array.isArray(p.colors) ||
      p.colors.length < 1 ||
      p.colors.length > 16 ||
      p.colors.some((c) => typeof c !== "string" || !/^#[0-9A-F]{6}$/.test(c))
    )
      throw new Error("配色格式无效");
    if (p.preset !== undefined) {
      const preset = catalog.palettes.find((c) => c.name === p.preset);
      if (
        !preset ||
        p.name !== preset.name ||
        JSON.stringify(p.colors) !== JSON.stringify(preset.colors)
      )
        throw new Error("预设配色内容不匹配");
    }
  }
  if (value.sources !== undefined) {
    if (!Array.isArray(value.sources) || value.sources.length > 4)
      throw new Error("设置来源无效");
    const seen = new Set();
    for (const s of value.sources) {
      if (
        !object(s) ||
        !allowed(s, ["node_id", "kind", "hash"]) ||
        typeof s.node_id !== "string" ||
        !/^[\da-f]{8}-(?:[\da-f]{4}-){3}[\da-f]{12}$/i.test(s.node_id) ||
        !cinemaKinds.includes(s.kind as CinemaKind) ||
        seen.has(s.kind) ||
        value[s.kind as string] === undefined ||
        typeof s.hash !== "string" ||
        !/^[\da-f]{64}$/.test(s.hash)
      )
        throw new Error("设置来源无效");
      seen.add(s.kind);
    }
  }
  return structuredClone(value) as CinemaKit;
}
export function cameraDefault(): CinemaCamera {
  return { body: "自动", lens: "自动", focal_length: "自动", aperture: "自动" };
}
export function kitKind(kit: CinemaKit): CinemaKind {
  const keys = cinemaKinds.filter((k) => kit[k] !== undefined);
  if (keys.length !== 1) throw new Error("独立工具箱节点必须只有一种设置");
  return keys[0];
}
export function cinemaSupports(kind: CinemaKind, target: string) {
  return target === "image"
    ? kind !== "motion"
    : target === "video"
      ? kind !== "camera"
      : false;
}
export function cinemaSummary(kit: CinemaKit, kind: CinemaKind) {
  if (kind === "camera")
    return (
      Object.values(kit.camera ?? {})
        .filter((v) => v !== "自动")
        .join(", ") || "自动"
    );
  if (kind === "palette") return kit.palette?.name ?? "配色";
  return kit[kind] ?? cinemaLabels[kind];
}

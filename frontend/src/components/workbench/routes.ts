import { episodes, projects, shots } from "./data";

export const projectSections = [
  ["", "项目概览"],
  ["script", "剧本"],
  ["bible", "设定集"],
  ["episodes/ep-01/assets", "单集资产"],
  ["episodes/ep-01/shots", "分镜"],
  ["episodes/ep-01/storyboard", "故事板"],
  ["episodes/ep-01/audio", "配音"],
  ["canvas", "创作画布"],
  ["media", "媒体库"],
  ["impact", "变更影响"],
  ["costs", "成本与预算"],
  ["settings", "项目设置"],
] as const;
export type ProjectPageKind =
  | "overview"
  | "script"
  | "bible"
  | "parse"
  | "assets"
  | "shots"
  | "shot"
  | "storyboard"
  | "audio"
  | "canvas"
  | "media"
  | "impact"
  | "costs"
  | "settings";
export type ProjectRoute = {
  project: (typeof projects)[number];
  kind: ProjectPageKind;
  episodeId?: string;
  shotId?: string;
};
export function resolveProjectRoute(
  projectId: string,
  section: string[] = [],
): ProjectRoute | null {
  const project = projects.find((item) => item.id === projectId);
  if (!project) return null;
  if (section.length === 0) return { project, kind: "overview" };
  const standalone = [
    "script",
    "bible",
    "canvas",
    "media",
    "impact",
    "costs",
    "settings",
  ];
  if (section.length === 1 && standalone.includes(section[0]))
    return { project, kind: section[0] as ProjectPageKind };
  if (
    section[0] !== "episodes" ||
    !episodes.some((item) => item.id === section[1])
  )
    return null;
  if (
    section.length === 3 &&
    ["parse", "assets", "shots", "storyboard", "audio"].includes(section[2])
  )
    return {
      project,
      kind: section[2] as ProjectPageKind,
      episodeId: section[1],
    };
  if (
    section.length === 4 &&
    section[2] === "shots" &&
    shots.some((item) => item.id === section[3])
  )
    return { project, kind: "shot", episodeId: section[1], shotId: section[3] };
  return null;
}
export function updatePreviewQuery(query: string, key: string, value: string) {
  const params = new URLSearchParams(query);
  if (value) params.set(key, value);
  else params.delete(key);
  return params.toString();
}

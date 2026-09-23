import type { CanvasRect } from "./canvas-geometry";

export type CanvasNode = CanvasRect & {
  id: string;
  title: string;
  subtitle: string;
  href?: string;
  kind: "project" | "episode" | GeneratedAPI.CanvasNode["kind"];
  saved?: GeneratedAPI.CanvasNode;
};

export function createNodes(
  project: API.ProjectResponse,
  episodes: API.EpisodeResponse[],
  document: GeneratedAPI.CanvasDocument,
): CanvasNode[] {
  const sorted = [...episodes].sort((a, b) => a.position - b.position || a.id.localeCompare(b.id));
  const savedByID = new Map(document.nodes.map((node) => [node.id, node]));
  const depth = (node: GeneratedAPI.CanvasNode): number => {
    let value = 0;
    let groupId = node.group_id;
    const seen = new Set([node.id]);
    while (groupId && !seen.has(groupId)) {
      seen.add(groupId);
      value += 1;
      groupId = savedByID.get(groupId)?.group_id;
    }
    return value;
  };
  const position = (node: GeneratedAPI.CanvasNode): { x: number; y: number } => {
    let x = node.x;
    let y = node.y;
    let groupId = node.group_id;
    const seen = new Set([node.id]);
    while (groupId && !seen.has(groupId)) {
      const parent = savedByID.get(groupId);
      if (!parent) break;
      seen.add(groupId);
      x += parent.x;
      y += parent.y;
      groupId = parent.group_id;
    }
    return { x, y };
  };
  const contextual: CanvasNode[] = [
    {
      id: `project:${project.id}`,
      kind: "project",
      title: project.name,
      subtitle: project.description || "项目创作起点",
      href: `/projects/${project.id}`,
      x: 0,
      y: Math.max(0, (sorted.length - 1) * 82),
      width: 300,
      height: 174,
    },
    ...sorted.map((episode, index) => ({
      id: `episode:${episode.id}`,
      kind: "episode" as const,
      title: episode.name,
      subtitle: `第 ${episode.position} 集 · ${Math.round(episode.target_duration_ms / 1000)} 秒`,
      href: `/studio/${episode.id}/script`,
      x: 440,
      y: index * 164,
      width: 276,
      height: 130,
    })),
  ];
  return [
    ...contextual,
    ...[...document.nodes].sort((left, right) => depth(left) - depth(right)).map((node) => {
      const world = position(node);
      return {
        id: node.id,
        kind: node.kind,
        title: node.title,
        subtitle: node.content || node.prompt || (node.media_version_id ? "已关联媒体版本" : "等待内容"),
        x: world.x,
        y: world.y,
        width: node.width,
        height: node.height,
        saved: node,
      };
    }),
  ];
}

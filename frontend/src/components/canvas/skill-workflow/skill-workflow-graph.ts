import type { CanvasEditorState } from "@/components/canvas-editor/lib/canvas-document-adapter.ts";
import { connectCanvasDocument } from "@/components/canvas-editor/lib/canvas-document-connections.ts";
import { CanvasNodeType } from "@/components/canvas-editor/types/canvas.ts";

export interface SkillWorkflowNode {
  id: string;
  skill: string;
  title: string;
  task: string;
  after: string[];
}
export interface SkillWorkflow {
  id: string;
  nodes: SkillWorkflowNode[];
  source?: { canvasId: string; revision: string; graphHash: string };
}
export interface SkillWorkflowTemplate {
  title: string;
  nodes: Omit<SkillWorkflowNode, "id">[];
}

const skillHeader = /^@skill ([a-z0-9]+(?:-[a-z0-9]+)*)\r?\n/;
const stageSpacing = { x: 320, y: 250 };

export function skillGroups({ nodes }: CanvasEditorState) {
  const groups = new Set(
    nodes.flatMap((node) =>
      node.type === CanvasNodeType.Text &&
      node.metadata?.groupId &&
      skillHeader.test(node.metadata.content ?? "")
        ? [node.metadata.groupId]
        : [],
    ),
  );
  return nodes.filter(
    (node) => node.type === CanvasNodeType.Group && groups.has(node.id),
  );
}

export function createSkillGroup(
  graph: CanvasEditorState,
  template: SkillWorkflowTemplate,
) {
  const editor: CanvasEditorState = { ...graph, nodes: [], connections: [] };
  const groupId = crypto.randomUUID();
  const startX =
    Math.max(0, ...graph.nodes.map((node) => node.position.x + node.width)) +
    stageSpacing.x / 2;
  const ids = new Map(
    template.nodes.map((node) => [node.skill, crypto.randomUUID()]),
  );
  const depth = new Map<string, number>();
  const rowByColumn = new Map<number, number>();
  editor.nodes.push({
    type: CanvasNodeType.Group,
    id: groupId,
    title: template.title,
    position: { x: startX, y: 0 },
    width: stageSpacing.x,
    height: stageSpacing.y,
  });
  for (const node of template.nodes) {
    const column =
      Math.max(-1, ...node.after.map((id) => depth.get(id) ?? -1)) + 1;
    const row = rowByColumn.get(column) ?? 0;
    rowByColumn.set(column, row + 1);
    depth.set(node.skill, column);
    editor.nodes.push({
      type: CanvasNodeType.Text,
      id: ids.get(node.skill)!,
      title: node.title,
      position: {
        x: startX + 36 + column * stageSpacing.x,
        y: 60 + row * stageSpacing.y,
      },
      width: 280,
      height: 190,
      metadata: { groupId, content: `@skill ${node.skill}\n${node.task}` },
    });
  }
  const group = editor.nodes.find((node) => node.id === groupId)!;
  group.width = (Math.max(...depth.values(), 0) + 1) * stageSpacing.x + 38;
  group.height = Math.max(...rowByColumn.values(), 1) * stageSpacing.y + 64;
  let linked = editor;
  for (const node of template.nodes) {
    for (const source of node.after) {
      const from = ids.get(source);
      const to = ids.get(node.skill);
      if (!from || !to) throw new Error("模板依赖引用了未知节点");
      linked = connectCanvasDocument(linked, from, to);
    }
  }
  return {
    groupId,
    graph: {
      ...graph,
      nodes: [...graph.nodes, ...linked.nodes],
      connections: [...graph.connections, ...linked.connections],
    },
  };
}

export function compileSkillGroup(
  graph: CanvasEditorState,
  groupId: string,
): SkillWorkflow {
  const children = graph.nodes.filter(
    (node) => node.metadata?.groupId === groupId,
  );
  const stageIds = new Set(
    children
      .filter(
        (node) =>
          node.type === CanvasNodeType.Text &&
          skillHeader.test(node.metadata?.content ?? ""),
      )
      .map((node) => node.id),
  );
  if (!stageIds.size) throw new Error("流程组中没有技能节点");
  const nodes = children
    .filter((node) => stageIds.has(node.id))
    .map((node) => {
      const content = node.metadata?.content ?? "";
      const header = skillHeader.exec(content);
      const skill = header?.[1];
      const task = content.slice(header?.[0].length).trim();
      if (!skill || !task)
        throw new Error(`请填写「${node.title}」的技能和任务`);
      return {
        id: node.id,
        skill,
        title: node.title,
        task,
        after: graph.connections
          .filter(
            (edge) =>
              edge.toNodeId === node.id && stageIds.has(edge.fromNodeId),
          )
          .sort(
            (left, right) =>
              (left.documentData?.order ?? 0) -
              (right.documentData?.order ?? 0),
          )
          .map((edge) => edge.fromNodeId),
      };
    });
  const remaining = new Set(nodes);
  const ready = new Set<string>();
  while (remaining.size > 0) {
    const count = remaining.size;
    for (const node of remaining) {
      if (!node.after.every((id) => ready.has(id))) continue;
      ready.add(node.id);
      remaining.delete(node);
    }
    if (remaining.size === count) throw new Error("技能流程存在循环依赖");
  }
  return { id: groupId, nodes };
}

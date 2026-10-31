import type { CanvasNodeData } from "../types/canvas";
export type ContainerKind = "group" | "stack";
export type ContainerBackground =
  "none" | "cyan" | "blue" | "purple" | "orange" | "yellow";
export function containerKind(node: CanvasNodeData): ContainerKind | null {
  if (node.type !== "group") return null;
  return node.serviceData?.config?.container?.kind === "stack"
    ? "stack"
    : "group";
}
export function containerDescendantIDs(
  nodes: CanvasNodeData[],
  id: string,
): Set<string> {
  const ids = new Set<string>();
  let changed = true;
  while (changed) {
    changed = false;
    for (const node of nodes) {
      const parent = node.metadata?.groupId;
      if (
        node.id !== id &&
        parent &&
        (parent === id || ids.has(parent)) &&
        !ids.has(node.id)
      ) {
        ids.add(node.id);
        changed = true;
      }
    }
  }
  return ids;
}
export function hiddenContainerMemberIDs(nodes: CanvasNodeData[]): Set<string> {
  const ids = new Set<string>();
  for (const node of nodes)
    if (containerKind(node) === "stack")
      for (const id of containerDescendantIDs(nodes, node.id)) ids.add(id);
  return ids;
}
export function selectedContainerRoots(
  nodes: CanvasNodeData[],
  selected: Set<string>,
): CanvasNodeData[] {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  return nodes.filter((node) => {
    if (!selected.has(node.id)) return false;
    let parent = node.metadata?.groupId;
    const visited = new Set<string>();
    while (parent && !visited.has(parent)) {
      if (selected.has(parent)) return false;
      visited.add(parent);
      parent = byID.get(parent)?.metadata?.groupId;
    }
    return true;
  });
}
export function createContainer(
  nodes: CanvasNodeData[],
  selected: Set<string>,
  container: CanvasNodeData,
  kind: ContainerKind,
): CanvasNodeData[] {
  const roots = selectedContainerRoots(nodes, selected);
  if (roots.length < 2) throw new Error("请选择至少两个节点");
  if (nodes.some((node) => node.id === container.id))
    throw new Error("容器身份已存在");
  const parent = roots[0].metadata?.groupId;
  const commonParent = roots.every((node) => node.metadata?.groupId === parent)
    ? parent
    : undefined;
  const next = {
    ...container,
    type: "group",
    metadata: { ...container.metadata, groupId: commonParent },
    serviceData: {
      ...container.serviceData,
      config: {
        ...container.serviceData?.config,
        collapsed: false,
        container: { version: 1, kind, background: "none" as const },
      },
    },
  };
  const ids = new Set(roots.map((node) => node.id));
  return [
    next,
    ...nodes.map((node) =>
      ids.has(node.id)
        ? { ...node, metadata: { ...node.metadata, groupId: container.id } }
        : node,
    ),
  ];
}
export function releaseContainers(
  nodes: CanvasNodeData[],
  selected: Set<string>,
): CanvasNodeData[] {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const removed = new Set(
    nodes
      .filter((node) => selected.has(node.id) && containerKind(node))
      .map((node) => node.id),
  );
  return nodes
    .filter((node) => !removed.has(node.id))
    .map((node) => {
      let parent = node.metadata?.groupId;
      const visited = new Set<string>();
      while (parent && removed.has(parent) && !visited.has(parent)) {
        visited.add(parent);
        parent = byID.get(parent)?.metadata?.groupId;
      }
      return parent === node.metadata?.groupId
        ? node
        : { ...node, metadata: { ...node.metadata, groupId: parent } };
    });
}
export function setContainerBackground(
  node: CanvasNodeData,
  background: ContainerBackground,
): CanvasNodeData {
  const kind = containerKind(node);
  if (!kind) throw new Error("所选节点不是容器");
  return {
    ...node,
    serviceData: {
      ...node.serviceData,
      config: {
        ...node.serviceData?.config,
        collapsed: node.documentData?.collapsed ?? false,
        container: { version: 1, kind, background },
      },
    },
  };
}

export function visibleContainerSelection(
  nodes: CanvasNodeData[],
  selected: Set<string>,
): Set<string> {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const visible = new Set<string>();
  for (const id of selected) {
    const node = byID.get(id);
    if (!node) continue;
    let target = node.id;
    let parent = node.metadata?.groupId;
    const visited = new Set<string>();
    while (parent && !visited.has(parent)) {
      visited.add(parent);
      const ancestor = byID.get(parent);
      if (!ancestor) break;
      if (containerKind(ancestor) === "stack") target = ancestor.id;
      parent = ancestor.metadata?.groupId;
    }
    visible.add(target);
  }
  return new Set(selectedContainerRoots(nodes, visible).map((node) => node.id));
}

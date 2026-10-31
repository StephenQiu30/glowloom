import {
  containerKind,
  hiddenContainerMemberIDs,
  createContainer,
  releaseContainers,
  selectedContainerRoots,
} from "./canvas-containers";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
  type ConnectionHandle,
} from "@/components/canvas/types/canvas";

export function nodeBounds(nodes: CanvasNodeData[]) {
  return nodes.reduce(
    (acc, node) => ({
      left: Math.min(acc.left, node.position.x),
      top: Math.min(acc.top, node.position.y),
      right: Math.max(acc.right, node.position.x + node.width),
      bottom: Math.max(acc.bottom, node.position.y + node.height),
    }),
    { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity },
  );
}

function containsCenter(group: CanvasNodeData, node: CanvasNodeData) {
  const centerX = node.position.x + node.width / 2;
  const centerY = node.position.y + node.height / 2;
  return (
    centerX >= group.position.x &&
    centerX <= group.position.x + group.width &&
    centerY >= group.position.y &&
    centerY <= group.position.y + group.height
  );
}

export function findGroupDropTarget(
  movedIds: Set<string>,
  nodes: CanvasNodeData[],
) {
  if (
    nodes.some(
      (node) => movedIds.has(node.id) && node.type === CanvasNodeType.Group,
    )
  )
    return null;
  const movingNodes = nodes.filter(
    (node) => movedIds.has(node.id) && node.type !== CanvasNodeType.Group,
  );
  if (!movingNodes.length) return null;
  const hidden = hiddenContainerMemberIDs(nodes);
  // Walk backwards instead of copying and reversing; this runs on every drag frame.
  for (let index = nodes.length - 1; index >= 0; index -= 1) {
    const group = nodes[index];
    if (
      containerKind(group) !== "group" ||
      movedIds.has(group.id) ||
      hidden.has(group.id)
    )
      continue;
    if (movingNodes.some((node) => containsCenter(group, node))) return group;
  }
  return null;
}

export function snapNodesIntoGroup(
  movedIds: Set<string>,
  nodes: CanvasNodeData[],
  group: CanvasNodeData,
) {
  const movingNodes = nodes.filter(
    (node) => movedIds.has(node.id) && node.type !== CanvasNodeType.Group,
  );
  if (!movingNodes.length) return nodes;
  const pad = 24;
  const bounds = nodeBounds(movingNodes);
  const left = group.position.x + pad;
  const top = group.position.y + pad;
  const right = group.position.x + group.width - pad;
  const bottom = group.position.y + group.height - pad;
  const dx =
    bounds.right - bounds.left > right - left
      ? left - bounds.left
      : bounds.left < left
        ? left - bounds.left
        : bounds.right > right
          ? right - bounds.right
          : 0;
  const dy =
    bounds.bottom - bounds.top > bottom - top
      ? top - bounds.top
      : bounds.top < top
        ? top - bounds.top
        : bounds.bottom > bottom
          ? bottom - bounds.bottom
          : 0;
  return nodes.map((node) => {
    if (!movedIds.has(node.id) || node.type === CanvasNodeType.Group)
      return node;
    return {
      ...node,
      position: { x: node.position.x + dx, y: node.position.y + dy },
      metadata: { ...node.metadata, groupId: group.id },
    };
  });
}

export const GROUP_WRAP_PADDING = 24;
export const GROUP_WRAP_TOP_PADDING = 52;

export function collectGroupMemberNodes(
  selectedIds: Set<string>,
  nodes: CanvasNodeData[],
) {
  return selectedContainerRoots(nodes, selectedIds);
}
export function getGroupWrapRect(members: CanvasNodeData[]) {
  const bounds = nodeBounds(members);
  return {
    x: bounds.left - GROUP_WRAP_PADDING,
    y: bounds.top - GROUP_WRAP_TOP_PADDING,
    width: bounds.right - bounds.left + GROUP_WRAP_PADDING * 2,
    height:
      bounds.bottom - bounds.top + GROUP_WRAP_TOP_PADDING + GROUP_WRAP_PADDING,
  };
}
export function canGroupSelectedNodes(
  selected: Set<string>,
  nodes: CanvasNodeData[],
) {
  return selectedContainerRoots(nodes, selected).length >= 2;
}
export function canUngroupSelectedNodes(
  selected: Set<string>,
  nodes: CanvasNodeData[],
) {
  return nodes.some(
    (node) =>
      selected.has(node.id) && (containerKind(node) || node.metadata?.groupId),
  );
}
export function applyGroupSelection(
  selected: Set<string>,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
  group: CanvasNodeData,
) {
  if (!canGroupSelectedNodes(selected, nodes)) return null;
  return {
    nodes: createContainer(nodes, selected, group, "group"),
    connections,
    selectedIds: [group.id],
  };
}
export function applyUngroupSelection(
  selected: Set<string>,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  if (!canUngroupSelectedNodes(selected, nodes)) return null;
  const containers = new Set(
    nodes
      .filter((node) => selected.has(node.id) && containerKind(node))
      .map((node) => node.id),
  );
  const next = releaseContainers(nodes, containers).map((node) =>
    selected.has(node.id) && !containers.size
      ? { ...node, metadata: { ...node.metadata, groupId: undefined } }
      : node,
  );
  return {
    nodes: next,
    connections: connections.filter(
      (edge) =>
        !containers.has(edge.fromNodeId) && !containers.has(edge.toNodeId),
    ),
    selectedIds: next
      .filter(
        (node) =>
          selected.has(node.id) ||
          node.metadata?.groupId !==
            nodes.find((old) => old.id === node.id)?.metadata?.groupId,
      )
      .map((node) => node.id),
  };
}

export function findContainingGroupId(
  node: CanvasNodeData,
  nodes: CanvasNodeData[],
) {
  const hidden = hiddenContainerMemberIDs(nodes);
  // Called once per moved node when a drag ends, so avoid copying the node list each time.
  for (let index = nodes.length - 1; index >= 0; index -= 1) {
    const group = nodes[index];
    if (
      containerKind(group) === "group" &&
      group.id !== node.id &&
      !hidden.has(group.id) &&
      containsCenter(group, node)
    )
      return group.id;
  }
  return undefined;
}

export function getConnectionTargetAnchor(
  node: CanvasNodeData,
  current: ConnectionHandle,
) {
  return {
    x:
      current.handleType === "source"
        ? node.position.x
        : node.position.x + node.width,
    y: node.position.y + node.height / 2,
  };
}

export function normalizeConnection(
  firstNodeId: string,
  secondNodeId: string,
  nodes: CanvasNodeData[],
  firstHandleType: "source" | "target",
) {
  const first = nodes.find((node) => node.id === firstNodeId);
  const second = nodes.find((node) => node.id === secondNodeId);
  if (!first || !second || first.id === second.id) return null;
  if (second.type === CanvasNodeType.Group) return null;
  if (
    first.type === CanvasNodeType.Config &&
    second.type === CanvasNodeType.Config
  )
    return null;
  if (second.type === CanvasNodeType.Config)
    return { fromNodeId: first.id, toNodeId: second.id };
  if (first.type === CanvasNodeType.Config && firstHandleType === "target")
    return { fromNodeId: second.id, toNodeId: first.id };
  if (first.type === CanvasNodeType.Config)
    return { fromNodeId: first.id, toNodeId: second.id };
  return { fromNodeId: first.id, toNodeId: second.id };
}

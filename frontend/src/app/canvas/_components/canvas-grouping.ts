import { isEditableNode, type Node } from "./editor-state";

// Adapted from the source editor's canvas-node-geometry group drop operations.
// World coordinates, nested ownership and server edit restrictions belong to this host.
export function reconcileGroupDrop(
  nodes: Node[],
  moved: ReadonlySet<string>,
): Node[] {
  const moving = nodes.filter((node) => moved.has(node.id));
  if (!moving.length || moving.some((node) => !isEditableNode(node)))
    return nodes;
  // A moving group keeps its complete descendant graph; never reparent its children.
  if (moving.some((node) => node.node_type === "group")) return nodes;
  const roots = moving.filter(
    (node) => !node.parent_id || !moved.has(node.parent_id),
  );
  const groups = nodes
    .filter(
      (node) =>
        node.node_type === "group" &&
        !moved.has(node.id) &&
        isEditableNode(node) &&
        !node.config.collapsed &&
        roots.every((member) => {
          const x = member.x + (member.width ?? 288) / 2;
          const y = member.y + (member.height ?? 200) / 2;
          return (
            x >= node.x &&
            x <= node.x + (node.width ?? 288) &&
            y >= node.y &&
            y <= node.y + (node.height ?? 200)
          );
        }),
    )
    .sort(
      (a, b) =>
        (a.width ?? 288) * (a.height ?? 200) -
          (b.width ?? 288) * (b.height ?? 200) || b.z_index - a.z_index,
    );
  const parent = groups[0]?.id;
  const ids = new Set(roots.map((node) => node.id));
  return nodes.map((node) =>
    ids.has(node.id) && node.parent_id !== parent
      ? { ...node, parent_id: parent }
      : node,
  );
}

// Group frame and connection anchors from the source node-geometry lib.
// Membership uses parent_id rather than a separate group metadata field.
import type { Node } from "../document";

export const GROUP_WRAP_PADDING = 24;
export const GROUP_WRAP_TOP_PADDING = 52;

type Box = Pick<Node, "x" | "y" | "width" | "height">;

export function nodeBounds(nodes: Box[]) {
  return nodes.reduce(
    (acc, node) => ({
      left: Math.min(acc.left, node.x),
      top: Math.min(acc.top, node.y),
      right: Math.max(acc.right, node.x + (node.width ?? 288)),
      bottom: Math.max(acc.bottom, node.y + (node.height ?? 200)),
    }),
    { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity },
  );
}

export function groupWrapRect(members: Box[]) {
  const bounds = nodeBounds(members);
  return {
    x: bounds.left - GROUP_WRAP_PADDING,
    y: bounds.top - GROUP_WRAP_TOP_PADDING,
    width: bounds.right - bounds.left + GROUP_WRAP_PADDING * 2,
    height:
      bounds.bottom - bounds.top + GROUP_WRAP_TOP_PADDING + GROUP_WRAP_PADDING,
  };
}

export function connectionAnchor(node: Box, handle: "source" | "target") {
  const width = node.width ?? 288;
  const height = node.height ?? 200;
  return {
    x: handle === "source" ? node.x : node.x + width,
    y: node.y + height / 2,
  };
}

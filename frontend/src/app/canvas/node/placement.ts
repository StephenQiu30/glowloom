import type { Node } from "../document";

/** Keep the proposed column, and find a free row without covering existing content. */
export function placeNode(node: Node, occupied: Node[]): Node {
  const x = Math.max(-1e6, Math.min(1e6, node.x));
  const width = node.width ?? 320;
  const height = node.height ?? 320;
  const obstacles = occupied.filter(
    (other) =>
      other.node_type !== "group" &&
      x < other.x + (other.width ?? 320) + 24 &&
      x + width + 24 > other.x,
  );
  const candidates = [node.y];
  for (let i = 1; i <= occupied.length + 1; i++)
    candidates.push(node.y + (height + 80) * i, node.y - (height + 80) * i);
  const y = candidates.find(
    (value) =>
      value >= -1e6 &&
      value <= 1e6 &&
      !obstacles.some(
        (other) =>
          value < other.y + (other.height ?? 320) + 24 &&
          value + height + 24 > other.y,
      ),
  );
  if (y === undefined) throw new Error("附近没有放置空间，请先调整节点布局。");
  return { ...node, x, y };
}

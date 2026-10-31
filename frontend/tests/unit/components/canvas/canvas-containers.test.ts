import { expect, it } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  containerKind,
  createContainer,
  releaseContainers,
  containerDescendantIDs,
  hiddenContainerMemberIDs,
} from "@/components/canvas/lib/canvas-containers";
const node = (kind = "text", parent?: string) =>
  createCanvasNode(
    kind,
    { x: 100, y: 200 },
    parent ? { groupId: parent } : undefined,
  );
it("typed stack preserves members and edges while only hiding descendants", () => {
  const a = node(),
    b = node();
  const stack = node("group");
  const result = createContainer([a, b], new Set([a.id, b.id]), stack, "stack");
  expect(result).toHaveLength(3);
  expect(containerKind(result.find((n) => n.id === stack.id)!)).toBe("stack");
  expect(hiddenContainerMemberIDs(result)).toEqual(new Set([a.id, b.id]));
  expect(result.find((n) => n.id === a.id)?.position).toEqual(a.position);
  expect(
    containerKind({
      ...node("group"),
      documentData: { collapsed: true } as never,
    }),
  ).toBe("group");
});
it("nested grouping keeps the inner container and never reparents selected descendants twice", () => {
  const inner = node("group"),
    child = node("text", inner.id),
    other = node(),
    outer = node("group");
  const result = createContainer(
    [inner, child, other],
    new Set([inner.id, child.id, other.id]),
    outer,
    "group",
  );
  expect(result).toHaveLength(4);
  expect(result.find((n) => n.id === child.id)?.metadata?.groupId).toBe(
    inner.id,
  );
  expect(containerDescendantIDs(result, outer.id)).toEqual(
    new Set([inner.id, child.id, other.id]),
  );
  const released = releaseContainers(result, new Set([outer.id]));
  expect(
    released.find((n) => n.id === inner.id)?.metadata?.groupId,
  ).toBeUndefined();
  expect(released.find((n) => n.id === child.id)?.metadata?.groupId).toBe(
    inner.id,
  );
});
it("unstacking promotes direct children to its parent and preserves original coordinates", () => {
  const outer = node("group"),
    stack = node("group", outer.id),
    a = node("text", stack.id),
    b = node("image", stack.id);
  const released = releaseContainers([outer, stack, a, b], new Set([stack.id]));
  expect(released.map((n) => n.id)).toEqual([outer.id, a.id, b.id]);
  expect(released.find((n) => n.id === a.id)).toMatchObject({
    position: a.position,
    metadata: { groupId: outer.id },
  });
});

it("copies a nested stack with distinct container identities and the same fixed content", async () => {
  const { captureCanvasSelection, instantiateCanvasClipboard } =
    await import("@/components/canvas/lib/canvas-clipboard");
  const outer = node("group"),
    inner = node("group"),
    a = node(),
    b = node(),
    other = node();
  const stacked = createContainer(
    [a, b],
    new Set([a.id, b.id]),
    inner,
    "stack",
  );
  const tree = createContainer(
    [...stacked, other],
    new Set([inner.id, a.id, other.id]),
    outer,
    "group",
  );
  const copied = instantiateCanvasClipboard(
    captureCanvasSelection(tree, [], new Set([outer.id])),
    { x: 50, y: 100 },
  );
  expect(copied.nodes).toHaveLength(5);
  expect(new Set(copied.nodes.map((n) => n.id)).size).toBe(5);
  const copiedStack = copied.nodes.find((n) => containerKind(n) === "stack")!;
  expect(copiedStack.id).not.toBe(inner.id);
  expect(containerDescendantIDs(copied.nodes, copiedStack.id).size).toBe(2);
  expect(hiddenContainerMemberIDs(copied.nodes).size).toBe(2);
  expect(
    copied.nodes.find((n) => n.title.startsWith(a.title) && n.type === "text")
      ?.position.x,
  ).toBe(a.position.x + 50);
});

it("never drops visible nodes into a hidden ordinary group inside a stack", async () => {
  const { findGroupDropTarget, findContainingGroupId } =
    await import("@/components/canvas/lib/canvas-node-geometry");
  const inner = node("group"),
    leaf = node("text", inner.id),
    sibling = node(),
    stack = node("group");
  const tree = createContainer(
    [inner, leaf, sibling],
    new Set([inner.id, sibling.id]),
    stack,
    "stack",
  );
  const moving = node();
  expect(
    findGroupDropTarget(new Set([moving.id]), [...tree, moving]),
  ).toBeNull();
  expect(findContainingGroupId(moving, [...tree, moving])).toBeUndefined();
});

it("maps hidden selection to its visible stack after undo or a remote tree update", async () => {
  const { visibleContainerSelection } =
    await import("@/components/canvas/lib/canvas-containers");
  const a = node(),
    b = node(),
    stack = node("group");
  const tree = createContainer([a, b], new Set([a.id, b.id]), stack, "stack");
  expect(
    visibleContainerSelection(tree, new Set([a.id, b.id, "deleted"])),
  ).toEqual(new Set([stack.id]));
  expect(
    visibleContainerSelection(
      releaseContainers(tree, new Set([stack.id])),
      new Set([a.id, b.id]),
    ),
  ).toEqual(new Set([a.id, b.id]));
});

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { changeCanvasColorTags } from "@/components/canvas/lib/canvas-color-tags";
import { CanvasColorTags } from "@/components/canvas/controls/canvas-color-tags";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  serviceGraph,
  serviceDocumentFromEditor,
} from "@/components/canvas/service-graph";

function nodes() {
  const a = createCanvasNode("text", { x: 0, y: 0 });
  const b = createCanvasNode("image", { x: 400, y: 0 });
  const c = createCanvasNode("text", { x: 800, y: 0 });
  a.metadata = { ...a.metadata, textBackground: "orange", colorTags: ["cyan"] };
  b.metadata = { ...b.metadata, colorTags: ["purple"] };
  return [a, b, c];
}
describe("canvas independent color tags", () => {
  it("batch adds missing tags and preserves other colors and unselected nodes", () => {
    const original = nodes();
    const selected = new Set(original.slice(0, 2).map((n) => n.id));
    const added = changeCanvasColorTags(original, selected, "cyan");
    expect(added[0]).toBe(original[0]);
    expect(added[1].metadata?.colorTags).toEqual(["purple", "cyan"]);
    expect(added[2]).toBe(original[2]);
    const removed = changeCanvasColorTags(added, selected, "cyan");
    expect(removed[0].metadata?.colorTags).toEqual([]);
    expect(removed[0].metadata?.textBackground).toBe("orange");
    expect(removed[1].metadata?.colorTags).toEqual(["purple"]);
    expect(original[1].metadata?.colorTags).toEqual(["purple"]);
    expect(
      changeCanvasColorTags(removed, selected, null)[1].metadata?.colorTags,
    ).toEqual([]);
  });
  it("saves and reopens tags independently of content and background", () => {
    const dto: API.DocumentDTO = {
      id: "11111111-1111-4111-8111-111111111111",
      project_id: "22222222-2222-4222-8222-222222222222",
      revision: "1",
      schema_version: 1,
      operation_version: 1,
      permissions: { read: true, write: true },
      nodes: [],
      edges: [],
    };
    const graph = serviceGraph(dto, dto.id!, dto.project_id!);
    graph.nodes = nodes();
    const saved = serviceDocumentFromEditor(dto, graph);
    const reopened = serviceGraph(saved, dto.id!, dto.project_id!);
    expect(reopened.nodes[0].metadata).toMatchObject({
      colorTags: ["cyan"],
      textBackground: "orange",
    });
    expect(serviceDocumentFromEditor(saved, reopened)).toEqual(saved);
  });
  it("exposes mixed state and clear without activating canvas gestures", () => {
    const change = vi.fn();
    const pointer = vi.fn();
    render(
      <div onPointerDown={pointer}>
        <CanvasColorTags nodes={nodes().slice(0, 2)} onChange={change} />
      </div>,
    );
    fireEvent.click(screen.getByRole("button", { name: "添加标签" }));
    const cyan = screen.getByRole("checkbox", { name: "添加“青绿色”" });
    expect(cyan.getAttribute("aria-checked")).toBe("mixed");
    fireEvent.pointerDown(cyan);
    fireEvent.click(cyan);
    expect(pointer).not.toHaveBeenCalled();
    expect(change).toHaveBeenCalledWith("cyan");
    fireEvent.click(screen.getByRole("button", { name: "全部清空" }));
    expect(change).toHaveBeenCalledWith(null);
  });
});

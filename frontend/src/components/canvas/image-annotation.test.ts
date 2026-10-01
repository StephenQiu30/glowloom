import { describe, expect, it } from "vitest";
import { annotationHistory, normalizeAnnotationRect } from "./image-annotation";
describe("图片标注编辑历史", () => {
  it("反向拖拽仍生成正向非负矩形", () =>
    expect(
      normalizeAnnotationRect({ x: 0.8, y: 0.7 }, { x: 0.2, y: 0.1 }),
    ).toEqual({
      x: 0.2,
      y: 0.1,
      width: expect.closeTo(0.6),
      height: expect.closeTo(0.6),
    }));
  it("撤销后重做保留原笔迹，分支编辑清除旧重做", () => {
    const dot = {
      type: "brush" as const,
      color: "#ff0000",
      size: 4,
      points: [{ x: 0.4, y: 0.5 }],
    };
    const text = {
      type: "text" as const,
      color: "#ffffff",
      size: 30,
      x: 0.1,
      y: 0.2,
      text: "角色 A",
    };
    const original = annotationHistory.push(annotationHistory.empty(), dot),
      undone = annotationHistory.undo(original);
    expect(annotationHistory.redo(undone)).toEqual(original);
    expect(original.items).toEqual([dot]);
    expect(annotationHistory.push(undone, text).redo).toEqual([]);
  });
});

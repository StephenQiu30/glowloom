import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasDocumentNodePanel } from "@/components/canvas/nodes/canvas-document-node-panel";
import type { CanvasDocumentIntegration } from "@/components/canvas/types/canvas-document-integration";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
vi.mock("@/components/canvas/runtime/hooks/use-copy-text", () => ({
  useCopyText: () => vi.fn(),
}));
vi.mock("@/components/canvas/parameters/canvas-model-select", () => ({
  CanvasModelSelect: () => null,
}));
vi.mock("@/components/canvas/parameters/canvas-generation-parameters", () => ({
  CanvasGenerationParameters: () => null,
}));
vi.mock("@/components/canvas/hooks/use-canvas-reference-previews", () => ({
  useCanvasReferencePreviews: () => ({ references: [] }),
}));
vi.mock("@/components/canvas/nodes/canvas-node-reference-bar", () => ({
  CanvasNodeReferenceBar: () => null,
}));
vi.mock(
  "@/components/canvas/runtime/components/editor/rich-text/rich-text-input",
  () => ({ RichTextInput: () => <div>提示词编辑器</div> }),
);
afterEach(cleanup);
const node: CanvasNodeData = {
  id: "node",
  type: "image",
  title: "图片 1",
  position: { x: 0, y: 0 },
  width: 320,
  height: 320,
  metadata: { prompt: "一片森林" },
  documentData: {
    id: "node",
    type: "image",
    nodeTypeVersion: 1,
    title: "图片 1",
    position: { x: 0, y: 0 },
    width: 320,
    height: 320,
    zIndex: 1,
    parameters: {
      modelSelection: { connectionId: "connection", modelId: "model" },
    },
  },
};
const generation: CanvasDocumentIntegration["generation"] = {
  ready: true,
  runs: [],
  submitting: [],
  generate: vi.fn(),
  refresh: vi.fn(),
};
it("未注入正式生成服务时即使选了模型也禁用，明确说明原因", () => {
  const submit = vi.fn();
  render(
    <CanvasDocumentNodePanel
      node={node}
      onChange={vi.fn()}
      onClose={vi.fn()}
      onGenerate={submit}
    />,
  );
  const button = screen.getByRole("button", { name: "生成" });
  expect(button.hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("status").textContent).toContain("生成服务尚未接入");
  fireEvent.click(button);
  expect(submit).not.toHaveBeenCalled();
});
it("就绪服务只允许有效提示词提交，空格不能触发任务", () => {
  const submit = vi.fn();
  const view = render(
    <CanvasDocumentNodePanel
      node={{ ...node, metadata: { prompt: "   " } }}
      generation={generation}
      onChange={vi.fn()}
      onClose={vi.fn()}
      onGenerate={submit}
    />,
  );
  expect(
    screen.getByRole("button", { name: "生成" }).hasAttribute("disabled"),
  ).toBe(true);
  view.rerender(
    <CanvasDocumentNodePanel
      node={node}
      generation={generation}
      onChange={vi.fn()}
      onClose={vi.fn()}
      onGenerate={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "生成" }));
  expect(submit).toHaveBeenCalledOnce();
});

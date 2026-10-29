import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useCanvasFilePreviews } from "@/components/canvas/hooks/use-canvas-file-previews";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
const preview = vi.hoisted(() => vi.fn());
vi.mock("@/components/canvas/lib/canvas-file-preview", () => ({
  readCanvasFilePreview: preview,
}));
vi.mock(
  "@/components/canvas/runtime/components/feedback/error-notification",
  () => ({ notifyError: vi.fn() }),
);
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
const node = () =>
  ({
    id: "result",
    type: "video",
    title: "成片",
    metadata: { status: "idle", content: "" },
    documentData: {
      fileReferences: [
        {
          referenceKey: "asset",
          fileId: "asset",
          version: "1",
          purpose: "CONTENT",
        },
      ],
    },
  }) as CanvasNodeData;
function Harness() {
  const [nodes, setNodes] = useState([node()]);
  useCanvasFilePreviews(nodes, true, setNodes, {
    projectId: "project",
    canvasId: "canvas",
    bindingRevision: 1,
  });
  return (
    <>
      <p>{nodes[0].metadata?.status}</p>
      <output>{nodes[0].metadata?.content}</output>
      <button onClick={() => setNodes([node()])}>重新投影</button>
    </>
  );
}
it("marks an authorized restored preview ready and preserves that status after graph reprojection", async () => {
  preview.mockResolvedValue("https://media.example/confirmed.mp4");
  render(<Harness />);
  await screen.findByText("success");
  expect(screen.queryByText("idle")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重新投影" }));
  await screen.findByText("success");
  expect(preview).toHaveBeenCalledTimes(1);
  expect(screen.getByText("https://media.example/confirmed.mp4")).toBeTruthy();
});
it("does not claim readiness when the authorization preview failed", async () => {
  preview.mockRejectedValue(new Error("unavailable"));
  render(<Harness />);
  await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
  expect(screen.getByText("idle")).toBeTruthy();
  expect(screen.queryByText("success")).toBeNull();
});

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConnectionCreateMenu } from "@/components/canvas/controls/canvas-create-menus";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";

afterEach(cleanup);
const node = (id: string, type: string) =>
  ({ id, type, title: id, position: { x: 0, y: 0 } }) as CanvasNodeData;
const pending = (handleType: "source" | "target") => ({
  position: { x: 0, y: 0 },
  connection: { nodeId: "source", handleType },
});
it("blocks mixed sources when the selected audio model rejects any source without creating a node", () => {
  const onCreate = vi.fn();
  const sources = [node("video", "video"), node("audio", "audio")];
  const previewAudio = vi.fn(() => {
    throw new Error("当前模型不支持视频参考");
  });
  render(
    <ConnectionCreateMenu
      pending={pending("source")}
      existingType="video"
      existingNode={sources[0]}
      existingNodes={sources}
      previewAudio={previewAudio}
      onCreate={onCreate}
      onClose={() => {}}
    />,
  );
  const audio = screen.getByRole("option", { name: /音频/ });
  expect(audio.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(audio);
  expect(onCreate).not.toHaveBeenCalled();
  expect(previewAudio).toHaveBeenCalledWith(sources);
});
it("allows a future audio model that explicitly accepts video references", () => {
  const onCreate = vi.fn();
  render(
    <ConnectionCreateMenu
      pending={pending("source")}
      existingType="video"
      existingNode={node("video", "video")}
      previewAudio={() => undefined}
      onCreate={onCreate}
      onClose={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("option", { name: "音频" }));
  expect(onCreate).toHaveBeenCalledWith("audio");
});
it("checks a new source against the existing audio target draft for its left entry", () => {
  const target = node("source", "audio");
  const previewAudio = vi.fn(
    (sources: CanvasNodeData[], received?: CanvasNodeData) => {
      expect(received).toBe(target);
      if (sources[0].type === "video") throw new Error("此模型不支持视频");
    },
  );
  render(
    <ConnectionCreateMenu
      pending={pending("target")}
      existingType="audio"
      existingNode={target}
      previewAudio={previewAudio}
      onCreate={() => {}}
      onClose={() => {}}
    />,
  );
  expect(
    screen.getByRole("option", { name: /视频/ }).getAttribute("aria-disabled"),
  ).toBe("true");
  expect(
    screen.getByRole("option", { name: "图片" }).getAttribute("aria-disabled"),
  ).not.toBe("true");
});

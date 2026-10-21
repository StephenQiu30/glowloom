import { createRef } from "react";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { InfiniteCanvas } from "@/components/canvas/infinite-canvas";
import {
  cropDataUrl,
  resolveUpscaleSize,
} from "@/components/canvas/image-data";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("滚轮缩放固定指针下的世界坐标，输入区域不触发缩放", () => {
  const change = vi.fn();
  const { getByTestId, getByRole } = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 20, y: 30, k: 1 }}
      tool="select"
      onViewportChange={change}
    >
      <textarea data-canvas-no-zoom />
    </InfiniteCanvas>,
  );
  fireEvent.wheel(getByTestId("canvas-surface"), {
    deltaY: -100,
    clientX: 220,
    clientY: 130,
  });
  const next = change.mock.calls[0][0];
  expect((220 - next.x) / next.k).toBeCloseTo(200);
  expect((130 - next.y) / next.k).toBeCloseTo(100);
  change.mockClear();
  fireEvent.wheel(getByRole("textbox"), { deltaY: -100 });
  expect(change).not.toHaveBeenCalled();
});
it("本地缩放保持比例；坏图片明确失败而不永久挂起", async () => {
  expect(resolveUpscaleSize(800, 400, 2048)).toEqual({
    width: 2048,
    height: 1024,
  });
  vi.stubGlobal(
    "Image",
    class {
      onerror?: () => void;
      set src(_s: string) {
        this.onerror?.();
      }
    },
  );
  await expect(cropDataUrl("broken")).rejects.toThrow("图片无法解码");
});

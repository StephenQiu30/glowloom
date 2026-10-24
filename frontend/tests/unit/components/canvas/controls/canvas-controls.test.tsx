import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { CanvasToolbar } from "@/components/canvas/controls/canvas-toolbar";
import { CanvasZoomControls } from "@/components/canvas/controls/canvas-zoom-controls";

vi.mock("@/components/canvas/lib/node-registry", () => ({
  useNodeRegistryVersion: () => 0,
  listNodeDefinitions: () => [
    { type: "text", title: "文本" },
    { type: "image", title: "图片" },
    { type: "config", title: "配置" },
  ],
}));
vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (
    select: (state: { theme: "dark"; setTheme: () => void }) => unknown,
  ) => select({ theme: "dark", setTheme: vi.fn() }),
}));
afterEach(cleanup);

it("合并画布入口后保留撤销、配置节点和只读限制", () => {
  const create = vi.fn();
  const undo = vi.fn();
  const props = {
    selectedCount: 1,
    canvasTool: "select" as const,
    canUndo: true,
    canRedo: false,
    backgroundMode: "dots" as const,
    showImageInfo: false,
    onCreateNode: create,
    onUndo: undo,
    onRedo: vi.fn(),
    onUpload: vi.fn(),
    onBrowseAssets: vi.fn(),
    onDelete: vi.fn(),
    onClear: vi.fn(),
    onCanvasToolChange: vi.fn(),
    onBackgroundModeChange: vi.fn(),
    onShowImageInfoChange: vi.fn(),
  };
  const { rerender } = render(
    <TooltipProvider>
      <CanvasToolbar {...props} />
    </TooltipProvider>,
  );
  expect(screen.queryByRole("button", { name: "添加节点" })).toBeNull();
  expect(screen.queryByRole("button", { name: "画布外观" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "更多画布操作" }));
  fireEvent.click(screen.getByRole("button", { name: "撤销" }));
  expect(undo).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "添加配置节点" }));
  expect(create).toHaveBeenCalledWith("config");
  expect(screen.getByRole("switch", { name: "图片信息" })).toBeTruthy();
  rerender(
    <TooltipProvider>
      <CanvasToolbar {...props} canEdit={false} />
    </TooltipProvider>,
  );
  expect(
    (screen.getByRole("button", { name: "添加配置节点" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    (screen.getByRole("button", { name: "删除选中" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

it("缩放菜单保留吸附开关和适应画布，移除重复视图菜单", () => {
  const snap = vi.fn();
  const fit = vi.fn();
  render(
    <TooltipProvider>
      <CanvasZoomControls
        scale={1}
        onScaleChange={vi.fn()}
        onFit={fit}
        isMiniMapOpen={false}
        onToggleMiniMap={vi.fn()}
        showConnections
        onToggleConnections={vi.fn()}
        snapToGrid={false}
        onToggleSnap={snap}
      />
    </TooltipProvider>,
  );
  expect(screen.queryByRole("button", { name: "视图设置" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "缩放选项，当前 100%" }));
  fireEvent.click(screen.getByRole("button", { name: "网格吸附" }));
  expect(snap).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "适应画布" }));
  expect(fit).toHaveBeenCalledOnce();
});

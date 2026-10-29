import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasDirectorWorkspace } from "@/components/canvas/director/canvas-director-workspace";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
import type { DirectorViewport } from "@/components/canvas/director/director-viewport";

const captured = vi.hoisted(() => ({
  viewport: null as ComponentProps<typeof DirectorViewport> | null,
}));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => true }));
// This regression exercises the workspace playback clock, not Radix focus layout.
vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? children : null,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: ReactNode }) => <p>{children}</p>,
}));
vi.mock("@/api/canvases", () => ({
  readDirectorRecovery: vi.fn().mockResolvedValue({ draft: null }),
}));
vi.mock("@/components/canvas/director/director-project-assets", () => ({
  DirectorProjectAssets: () => null,
}));
vi.mock("@/components/canvas/director/director-viewport", () => ({
  DirectorViewport: (props: ComponentProps<typeof DirectorViewport>) => {
    captured.viewport = props;
    return null;
  },
}));
vi.mock("@/components/canvas/director/director-capture", () => ({
  DirectorCapture: () => null,
  DirectorGallery: () => null,
}));
vi.mock("@/components/canvas/director/director-view-preferences", () => ({
  directorHelpSeen: () => true,
  markDirectorHelpSeen: () => {},
}));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it.each([true, false])(
  "previews the entire extended path without saving and restores the original duration on cancel (loop=%s)",
  async (loop) => {
    let frame: FrameRequestCallback | undefined;
    vi.spyOn(performance, "now").mockReturnValue(0);
    vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation(
      (callback: FrameRequestCallback) => {
        frame = callback;
        return 1;
      },
    );
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {});
    const scene = { ...emptyDirectorConfig(), mode: "dynamic" as const };
    scene.shots[0].duration = 1;
    const snapshot = {
      writable: true,
      migration: false,
      document: {
        nodes: [{ id: "director", serviceData: { config: { director: scene } } }],
      },
    };
    const client = {
      subscribe: () => () => {},
      getSnapshot: () => snapshot,
      getDocumentEpoch: () => "epoch",
      retainedDirector: () => null,
      changeDirector: vi.fn(),
    };
    render(
      <CanvasDirectorWorkspace
        client={
          client as unknown as ComponentProps<
            typeof CanvasDirectorWorkspace
          >["client"]
        }
        nodeID="director"
        canvasID="canvas"
        projectID="project"
      />,
    );
    await act(async () => {});
    if (!loop) fireEvent.click(screen.getByText("循环", { selector: "button" }));
    act(() =>
      captured.viewport!.onSelect({ kind: "camera", id: scene.cameras[0].id }),
    );
    fireEvent.click(screen.getByText("添加运镜", { selector: "button" }));
    fireEvent.click(screen.getByText("预览", { selector: "button" }));
    expect(captured.viewport!.scene.shots[0].duration).toBe(4);
    act(() => frame!(1500));
    expect(captured.viewport!.time).toBeCloseTo(1.5);
    act(() => frame!(3500));
    expect(captured.viewport!.time).toBeCloseTo(3.5);
    expect(client.changeDirector).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("取消", { selector: "button" }));
    expect(captured.viewport!.scene.shots[0].duration).toBe(1);
    expect(captured.viewport!.scene.cameras[0].keyframes).toEqual([]);
    expect(captured.viewport!.time).toBeLessThanOrEqual(1);
    expect(client.changeDirector).not.toHaveBeenCalled();
  },
);

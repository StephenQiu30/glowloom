import { act, fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { DirectorViewport } from "@/components/canvas/director/director-viewport";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
const state = vi.hoisted(() => ({
  engines: [] as Array<Record<string, ReturnType<typeof vi.fn>>>,
  callbacks: [] as Array<{ onContextState: (value: boolean) => void }>,
}));
vi.mock("@/components/canvas/director/director-scene", () => ({
  disposeDirectorObject: vi.fn(),
  loadDirectorGLB: vi.fn(),
  DirectorScene: class {
    constructor(
      _host: unknown,
      callbacks: { onContextState: (value: boolean) => void },
    ) {
      const engine = Object.fromEntries(
        [
          "setViews",
          "setPlacement",
          "setTargetCamera",
          "setPreviewObject",
          "update",
          "focus",
          "dispose",
          "restoreDirectorView",
          "groundSelection",
          "exportVideo",
          "snapshotPoses",
          "snapshotNodePoses",
          "capture",
          "standardView",
        ].map((name) => [name, vi.fn()]),
      );
      engine.directorView = vi.fn(() => ({
        position: [3, 4, 5],
        target: [0, 1, 0],
        fov: 50,
      }));
      state.engines.push(engine);
      state.callbacks.push(callbacks);
      return engine;
    }
  },
}));
it("recreates a lost context, disposes the old runtime, and reapplies scene/view/target contracts", () => {
  const scene = emptyDirectorConfig(),
    camera = scene.cameras[0].id;
  const props = {
    scene,
    time: 0,
    selected: { kind: "camera" as const, id: camera },
    targetCameraID: camera,
    tool: "translate" as const,
    writable: true,
    projectID: "project",
    compact: false,
    focus: 0,
    views: {
      main: "director",
      preview: camera,
      visible: true,
      width: 320,
      split: true,
    },
    onSelect: vi.fn(),
    onGestureStart: vi.fn(),
    onTransform: vi.fn(),
    onError: vi.fn(),
  };
  const view = render(<DirectorViewport {...props} />);
  expect(state.engines).toHaveLength(1);
  act(() => state.callbacks[0].onContextState(true));
  expect(screen.getByRole("alert").textContent).toContain("场景数据已保留");
  fireEvent.click(screen.getByRole("button", { name: "恢复视口" }));
  expect(state.engines).toHaveLength(2);
  expect(state.engines[0].dispose).toHaveBeenCalledTimes(1);
  expect(state.engines[1].restoreDirectorView).toHaveBeenCalledWith({
    position: [3, 4, 5],
    target: [0, 1, 0],
    fov: 50,
  });
  expect(state.engines[1].setTargetCamera).toHaveBeenCalledWith(camera);
  expect(state.engines[1].setViews).toHaveBeenCalledWith(
    "director",
    camera,
    true,
    320,
    props.views,
  );
  expect(state.engines[1].update).toHaveBeenCalledWith(
    scene,
    0,
    props.selected,
    "translate",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "切换顶视图" }));
  expect(state.engines[1].standardView).toHaveBeenCalledWith("top");
  view.unmount();
  expect(state.engines[1].dispose).toHaveBeenCalledTimes(1);
});

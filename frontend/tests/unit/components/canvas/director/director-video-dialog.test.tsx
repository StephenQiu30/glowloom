import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DirectorVideoDialog } from "@/components/canvas/director/director-video-dialog";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";

afterEach(cleanup);
it("opens with the current shot and aspect after switching scenes", () => {
  const initial = { ...emptyDirectorConfig(), mode: "dynamic" as const };
  const props = {
    client: {} as CanvasYjsLiveSession,
    nodeID: "node",
    projectID: "project",
    exporter: vi.fn(),
    disabled: false,
    onBusy: vi.fn(),
  };
  const view = render(<DirectorVideoDialog {...props} scene={initial} />);
  const current = structuredClone(initial);
  current.cameras[0].id = "camera-current";
  current.cameras[0].name = "当前机位";
  current.shots[0].camera_id = "camera-current";
  current.shots[0].duration = 12;
  current.shots[0].fps = 60;
  current.aspect_ratio = "9:16";
  view.rerender(<DirectorVideoDialog {...props} scene={current} />);
  fireEvent.click(screen.getByRole("button", { name: "导出视频" }));
  expect(
    screen.getByRole("combobox", { name: "机位来源" }).textContent,
  ).toContain("当前机位");
  expect(screen.getByRole("combobox", { name: "画幅" }).textContent).toContain(
    "9:16",
  );
  expect(screen.getByRole("combobox", { name: "帧率" }).textContent).toContain(
    "60 fps",
  );
  expect(
    screen
      .getByRole("spinbutton", { name: "时长（秒）" })
      .getAttribute("value"),
  ).toBe("12");
});

it("uses all shot durations for sequence export and keeps manual overrides editable", async () => {
  const scene = { ...emptyDirectorConfig(), mode: "dynamic" as const };
  scene.shots[0].duration = 1;
  scene.shots.push({
    ...scene.shots[0],
    id: crypto.randomUUID(),
    name: "第二镜头",
    duration: 1,
  });
  const exporter = vi.fn().mockRejectedValue(new Error("test export stopped"));
  const client = {
    getSnapshot: () => ({
      writable: true,
      document: {
        nodes: [{ id: "node", serviceData: { config: { director: scene } } }],
      },
    }),
    getDocumentEpoch: () => "epoch",
    retainedDirector: () => null,
    waitForSaved: vi.fn().mockResolvedValue(undefined),
  } as unknown as CanvasYjsLiveSession;
  render(
    <DirectorVideoDialog
      client={client}
      nodeID="node"
      projectID="project"
      scene={scene}
      exporter={exporter}
      disabled={false}
      onBusy={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "导出视频" }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "机位来源" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "按分镜顺序" }));
  const duration = screen.getByRole("spinbutton", {
    name: "时长（秒）",
  }) as HTMLInputElement;
  expect(duration.value).toBe("2");
  fireEvent.change(duration, { target: { value: "1.5" } });
  fireEvent.click(screen.getByRole("button", { name: "开始导出" }));
  await vi.waitFor(() =>
    expect(exporter).toHaveBeenCalledWith(
      expect.objectContaining({ camera: "sequence", duration: 1.5 }),
      expect.any(AbortSignal),
      expect.any(Function),
    ),
  );
  await screen.findByRole("alert");
  fireEvent.keyDown(screen.getByRole("combobox", { name: "机位来源" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("option", { name: scene.cameras[0].name }),
  );
  expect(duration.value).toBe("1");
});

import { within } from "@testing-library/react";
import { render, findFeedback } from "../../../feedback";
import { afterEach, it, expect, vi } from "vitest";
import { cleanup, screen, fireEvent, waitFor } from "@testing-library/react";
import { CinemaKitPicker } from "@/components/canvas/cinema-kit/cinema-kit-picker";
import { ConnectionCreateMenu } from "@/components/canvas/controls/canvas-create-menus";
import { makeCinemaNode } from "@/components/canvas/cinema-kit/cinema-kit-graph";
import { cameraDefault } from "@/components/canvas/cinema-kit/cinema-kit-model";
import { registerBuiltinNodes } from "@/components/canvas/nodes/builtin-nodes";
registerBuiltinNodes();
afterEach(cleanup);
it("cycles four camera categories and only confirms the saved typed choice", async () => {
  const confirm = vi.fn(),
    close = vi.fn();
  render(<CinemaKitPicker kind="camera" onConfirm={confirm} onClose={close} />);
  fireEvent.click(screen.getByLabelText("下一相机"));
  fireEvent.click(screen.getByLabelText("下一焦距"));
  fireEvent.click(screen.getByLabelText("下一焦距"));
  fireEvent.click(screen.getByText("确认"));
  await waitFor(() => expect(close).toHaveBeenCalled());
  expect(confirm).toHaveBeenCalledWith({
    version: 1,
    camera: {
      body: "现代数字",
      lens: "自动",
      focal_length: "35mm",
      aperture: "自动",
    },
  });
});
it("keeps a failed save open with the draft available for retry", async () => {
  const close = vi.fn();
  render(
    <CinemaKitPicker
      kind="style"
      onConfirm={() => Promise.reject(new Error("保存未确认"))}
      onClose={close}
    />,
  );
  fireEvent.click(screen.getByText("确认"));
  await findFeedback();
  expect((await findFeedback()).textContent).toBe("保存未确认");
  expect(close).not.toHaveBeenCalled();
});
it("motion selection is single and clear disables confirm", async () => {
  render(
    <CinemaKitPicker kind="motion" onConfirm={vi.fn()} onClose={vi.fn()} />,
  );
  expect((screen.getByText("确认") as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("radio", { name: /迎面跟拍/ }));
  fireEvent.click(screen.getByRole("radio", { name: /侧面跟拍/ }));
  expect(
    screen
      .getByRole("radio", { name: /迎面跟拍/ })
      .getAttribute("aria-checked"),
  ).toBe("false");
  expect(
    screen
      .getByRole("radio", { name: /侧面跟拍/ })
      .getAttribute("aria-checked"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("radio", { name: "侧面跟拍" }));
  expect((screen.getByText("确认") as HTMLButtonElement).disabled).toBe(true);
});
it("keeps all seven connection choices visible and explains camera video restriction", async () => {
  const node = makeCinemaNode(
    { version: 1, camera: cameraDefault() },
    { x: 0, y: 0 },
    [],
  );
  render(
    <ConnectionCreateMenu
      pending={{
        position: { x: 0, y: 0 },
        connection: { nodeId: node.id, handleType: "source" },
      }}
      existingType="cinema_studio"
      existingNode={node}
      onCreate={vi.fn()}
      onClose={vi.fn()}
    />,
  );
  for (const name of [
    "文本",
    "图片",
    "视频",
    "音频",
    "时间线",
    "主体",
    "3D导演台",
  ])
    expect(screen.getByText(name)).toBeTruthy();
  expect(screen.getByText("暂不支持这些设置的模型")).toBeTruthy();
  expect(
    screen.getByRole("option", { name: /图片/ }).getAttribute("aria-disabled"),
  ).not.toBe("true");
});

it("previews a palette locally and confirms its typed colors without media URLs", async () => {
  const confirm = vi.fn();
  render(
    <CinemaKitPicker kind="palette" onConfirm={confirm} onClose={vi.fn()} />,
  );
  fireEvent.click(screen.getByRole("radio", { name: "沙漠史诗色调" }));
  expect(
    decodeURIComponent(
      screen.getByRole("img", { name: "沙漠史诗色调" }).getAttribute("src") ??
        "",
    ),
  ).toContain("/cinema/");
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() => expect(confirm).toHaveBeenCalled());
  const value = confirm.mock.calls[0][0];
  expect(value.palette.preset).toBe("沙漠史诗色调");
  expect(value.palette.colors).toHaveLength(8);
  expect(JSON.stringify(value)).not.toContain("/cinema/");
});

it("cycles camera choices with the keyboard and confirms a typed camera intent", async () => {
  const confirm = vi.fn();
  render(
    <CinemaKitPicker kind="camera" onConfirm={confirm} onClose={vi.fn()} />,
  );
  fireEvent.keyDown(
    within(screen.getByRole("radiogroup", { name: "焦距" })).getByRole(
      "radio",
      { name: "自动" },
    ),
    {
      key: "ArrowRight",
    },
  );
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() => expect(confirm).toHaveBeenCalled());
  expect(confirm.mock.calls[0][0].camera.focal_length).toBe("24mm");
});

it("selects a style through the shared toggle group without exposing neighbor previews", async () => {
  const confirm = vi.fn();
  render(
    <CinemaKitPicker kind="style" onConfirm={confirm} onClose={vi.fn()} />,
  );
  expect(screen.getAllByRole("img")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "下一风格" }));
  expect(
    screen
      .getByRole("radio", { name: "3D动画风" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() =>
    expect(confirm).toHaveBeenCalledWith({ version: 1, style: "3D动画风" }),
  );
});

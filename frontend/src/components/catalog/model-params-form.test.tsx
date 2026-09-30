import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { ModelParamsForm, type ParamField } from "./model-params-form";

afterEach(cleanup);

it("renders the selected model and mode, then drops stale values on switch", async () => {
  const onSubmit = vi.fn();
  const first: ParamField[] = [
    {
      field: "duration_ms",
      label: "时长",
      type: "integer",
      component: "segmented",
      enum: [5000, 10000],
      default: 5000,
      for_modes: ["omni_reference"],
    },
    {
      field: "resolution",
      label: "分辨率",
      type: "string",
      component: "select",
      enum: ["720p", "1080p"],
      default: "1080p",
    },
    {
      field: "camera_fixed",
      label: "固定镜头",
      type: "boolean",
      component: "switch",
      default: false,
    },
    {
      field: "seed",
      label: "随机种子",
      type: "integer",
      component: "input",
      min: 0,
      required: false,
    },
  ];
  const second: ParamField[] = [
    {
      field: "resolution",
      label: "分辨率",
      type: "string",
      component: "select",
      enum: ["2K"],
      default: "2K",
    },
    {
      field: "prompt",
      label: "提示词",
      type: "string",
      component: "textarea",
      required: true,
    },
  ];

  const view = render(
    <ModelParamsForm
      modelKey="ark"
      profileVersionId="ark-v1"
      mode="omni_reference"
      schema={first}
      onSubmit={onSubmit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "10 秒" }));
  fireEvent.change(screen.getByRole("combobox", { name: "分辨率" }), {
    target: { value: "720p" },
  });
  fireEvent.click(screen.getByRole("checkbox", { name: "固定镜头" }));
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  await waitFor(() =>
    expect(onSubmit).toHaveBeenLastCalledWith({
      duration_ms: 10000,
      resolution: "720p",
      camera_fixed: true,
    }),
  );

  view.rerender(
    <ModelParamsForm
      modelKey="ark"
      profileVersionId="ark-v1"
      mode="text2video"
      schema={first}
      onSubmit={onSubmit}
    />,
  );
  expect(screen.queryByRole("button", { name: "10 秒" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  await waitFor(() =>
    expect(onSubmit).toHaveBeenLastCalledWith({
      resolution: "1080p",
      camera_fixed: false,
    }),
  );

  view.rerender(
    <ModelParamsForm
      modelKey="minimax"
      profileVersionId="minimax-v1"
      mode="image2video"
      schema={second}
      onSubmit={onSubmit}
    />,
  );
  expect(screen.queryByRole("checkbox", { name: "固定镜头" })).toBeNull();
  expect(screen.queryByRole("button", { name: "10 秒" })).toBeNull();
  expect(
    (screen.getByRole("combobox", { name: "分辨率" }) as HTMLSelectElement)
      .value,
  ).toBe("2K");
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "请填写提示词",
  );
  expect(document.activeElement).toBe(
    screen.getByRole("textbox", { name: "提示词" }),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "提示词" }), {
    target: { value: "夜景" },
  });
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  await waitFor(() =>
    expect(onSubmit).toHaveBeenLastCalledWith({
      resolution: "2K",
      prompt: "夜景",
    }),
  );
});

it("keeps invalid integers inline and blocks submission", async () => {
  const onSubmit = vi.fn();
  const schema: ParamField[] = [
    {
      field: "seed",
      label: "随机种子",
      type: "integer",
      component: "input",
      min: 0,
      max: 10,
      required: true,
    },
  ];
  render(
    <ModelParamsForm
      modelKey="ark"
      profileVersionId="ark-v1"
      mode="image2video"
      schema={schema}
      onSubmit={onSubmit}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "请填写随机种子",
  );
  expect(onSubmit).not.toHaveBeenCalled();

  fireEvent.change(screen.getByRole("spinbutton", { name: "随机种子" }), {
    target: { value: "-1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "不能小于 0",
  );
  expect(onSubmit).not.toHaveBeenCalled();

  fireEvent.change(screen.getByRole("spinbutton", { name: "随机种子" }), {
    target: { value: "7" },
  });
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({ seed: 7 }));
});

it("submits slider and voice choices as typed values", async () => {
  const onSubmit = vi.fn();
  const schema: ParamField[] = [
    {
      field: "guidance",
      label: "引导强度",
      type: "number",
      component: "slider",
      min: 0,
      max: 1,
      default: 0.5,
    },
    {
      field: "voice",
      label: "音色",
      type: "string",
      component: "voice",
      enum: ["warm", "clear"],
      default: "warm",
    },
  ];
  render(
    <ModelParamsForm
      modelKey="tts"
      profileVersionId="tts-v1"
      mode="audio"
      schema={schema}
      onSubmit={onSubmit}
    />,
  );

  fireEvent.change(screen.getByRole("slider", { name: "引导强度" }), {
    target: { value: "0.8" },
  });
  fireEvent.change(screen.getByRole("combobox", { name: "音色" }), {
    target: { value: "clear" },
  });
  fireEvent.click(screen.getByRole("button", { name: "继续报价" }));
  await waitFor(() =>
    expect(onSubmit).toHaveBeenCalledWith({ guidance: 0.8, voice: "clear" }),
  );
});

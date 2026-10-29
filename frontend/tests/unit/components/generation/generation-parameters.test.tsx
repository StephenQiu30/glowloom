import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { GenerationParameters } from "@/components/generation/generation-parameters";

afterEach(cleanup);
it("Select 数字枚举通过真实选项交互，保留零值与参数类型", async () => {
  const change = vi.fn();
  render(
    <GenerationParameters
      fields={[
        {
          field: "seed",
          label: "随机种子",
          type: "integer",
          component: "select",
          enum: [0, 1, 2, 3, 4, 5, 6, 7],
        },
      ]}
      params={{}}
      maxOutputs={1}
      disabled={false}
      compact={false}
      onChange={change}
    />,
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "随机种子" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "0" }));
  expect(change).toHaveBeenCalledWith({ seed: 0 });
});

it("时长用秒编辑而保留毫秒协议和模型边界", () => {
  const change = vi.fn();
  render(
    <GenerationParameters
      fields={[
        {
          field: "duration_ms",
          label: "视频时长（毫秒）",
          type: "integer",
          component: "input",
          min: 4000,
          max: 30000,
          step: 1000,
        },
      ]}
      params={{ duration_ms: 5000 }}
      maxOutputs={1}
      disabled={false}
      compact
      onChange={change}
    />,
  );
  const input = screen.getByRole("spinbutton", { name: "时长（秒）" });
  expect(input).toHaveProperty("value", "5");
  expect(input.getAttribute("min")).toBe("4");
  expect(input.getAttribute("max")).toBe("30");
  expect(
    screen
      .getByRole("slider", { name: "时长（秒）" })
      .getAttribute("aria-valuenow"),
  ).toBe("5");
  fireEvent.change(input, { target: { value: "8" } });
  expect(change).toHaveBeenLastCalledWith({ duration_ms: 8000 });
  fireEvent.change(input, { target: { value: "" } });
  expect(change).toHaveBeenLastCalledWith({ duration_ms: null });
});

it("朗读音色使用模型默认值但不向创作者暴露内部ID输入框", () => {
  render(
    <GenerationParameters
      fields={[
        {
          field: "speaker",
          label: "音色",
          type: "string",
          component: "input",
          default: "provider_voice_key",
        },
      ]}
      params={{ speaker: "provider_voice_key" }}
      maxOutputs={1}
      disabled={false}
      compact
      onChange={() => {}}
    />,
  );
  expect(screen.getByRole("combobox", { name: "音色" }).textContent).toBe(
    "默认音色",
  );
  expect(screen.queryByRole("textbox", { name: "音色" })).toBeNull();
  expect(screen.queryByText("provider_voice_key")).toBeNull();
});

it("毫秒枚举按秒展示但提交原毫秒值", async () => {
  const change = vi.fn();
  render(
    <GenerationParameters
      fields={[
        {
          field: "duration_ms",
          label: "时长",
          type: "integer",
          component: "select",
          enum: [5000, 10000],
        },
      ]}
      params={{ duration_ms: 5000 }}
      maxOutputs={1}
      disabled={false}
      compact
      onChange={change}
    />,
  );
  expect(screen.getByRole("combobox", { name: "时长（秒）" }).textContent).toBe(
    "5",
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "时长（秒）" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "10" }));
  expect(change).toHaveBeenCalledWith({ duration_ms: 10000 });
});

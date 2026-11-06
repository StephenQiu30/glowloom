import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ModelPicker } from "@/components/generation/model-picker";

afterEach(cleanup);
const models: API.ModelResponse[] = [
  {
    id: "first",
    key: "first",
    display_name: "图像模型",
    provider_name: "供应商甲",
    status: "active",
    provider_status: "active",
    credential_present: true,
    current_version: { id: "first-version" },
  },
  {
    id: "second",
    key: "second",
    display_name: "停用模型",
    provider_name: "供应商乙",
    status: "disabled",
    provider_status: "active",
    credential_present: true,
    current_version: { id: "second-version" },
  },
];

it("键盘打开模型列表并选择真实模型", async () => {
  const select = vi.fn();
  render(
    <ModelPicker
      models={models}
      value={null}
      disabled={false}
      onSelect={select}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: "生成模型" });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  fireEvent.click(await screen.findByRole("option", { name: /图像模型/ }));
  expect(select).toHaveBeenCalledWith(models[0]);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
});

it("停用模型保留可见说明但不允许选择", async () => {
  const select = vi.fn();
  render(
    <ModelPicker
      models={models}
      value="first-version"
      disabled={false}
      onSelect={select}
    />,
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模型" }), {
    key: "ArrowDown",
  });
  const disabled = await screen.findByRole("option", { name: /停用模型/ });
  expect(disabled.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(disabled);
  expect(select).not.toHaveBeenCalled();
});

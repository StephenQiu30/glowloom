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

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  ModelSetup,
  modelConfiguration,
} from "@/app/models/_components/model-setup";

afterEach(cleanup);

describe("official model configuration controls", () => {
  it("preserves the saved advanced payload and switches modes through the labelled checkbox", () => {
    const limits = { max_outputs: 2, provider_endpoint: "/images" };
    render(
      <form aria-label="模型配置">
        <ModelSetup
          capability="image.generate"
          limits={limits}
          parameters={[]}
        />
      </form>,
    );
    const form = screen.getByRole("form", {
      name: "模型配置",
    }) as HTMLFormElement;
    const toggle = screen.getByRole("checkbox", {
      name: "编辑完整参数与素材规则",
    });
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    fireEvent.change(screen.getByLabelText("输入与输出限制"), {
      target: { value: JSON.stringify({ ...limits, max_outputs: 3 }) },
    });
    expect(modelConfiguration(new FormData(form), "image.generate")).toEqual({
      limits: { ...limits, max_outputs: 3 },
      param_schema: [],
    });

    fireEvent.click(toggle);
    expect(new FormData(form).get("setup_mode")).toBe("basic");
    expect(
      modelConfiguration(new FormData(form), "image.generate").limits,
    ).toMatchObject({
      max_outputs: 1,
    });
  });
});

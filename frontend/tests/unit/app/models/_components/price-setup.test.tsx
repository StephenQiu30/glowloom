import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { priceRule, PriceSetup } from "@/app/models/_components/model-setup";

afterEach(cleanup);

it("简单价格直接以元回填，不要求管理员编辑微元 JSON", () => {
  render(<PriceSetup unit="per_image" rule={{ base_micros: 120000 }} />);
  expect((screen.getByLabelText("价格（元）") as HTMLInputElement).value).toBe(
    "0.12",
  );
  expect(screen.queryByLabelText("每千输入 Token（元）")).toBeNull();
  expect(
    screen.queryByRole("textbox", { name: "人民币微元计费规则" }),
  ).toBeNull();
});

it("Token 计费只呈现输入与输出单价，保留已有价格", () => {
  render(
    <PriceSetup
      unit="per_1k_tokens"
      rule={{ input_micros_per_1k: 1000, output_micros_per_1k: 3000 }}
    />,
  );
  expect(
    (screen.getByLabelText("每千输入 Token（元）") as HTMLInputElement).value,
  ).toBe("0.001");
  expect(
    (screen.getByLabelText("每千输出 Token（元）") as HTMLInputElement).value,
  ).toBe("0.003");
  expect(screen.queryByLabelText("价格（元）")).toBeNull();
});

it("复杂规则按需编辑，保存仍保留完整原规则", () => {
  const rule = { base_micros: 120000, resolution_multipliers: { "4K": 2 } };
  render(<PriceSetup unit="per_image" rule={rule} />);
  expect(
    screen.queryByRole("textbox", { name: "人民币微元计费规则" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "高级计费规则" }));
  expect(
    JSON.parse(
      (
        screen.getByRole("textbox", {
          name: "人民币微元计费规则",
        }) as HTMLTextAreaElement
      ).value,
    ),
  ).toEqual(rule);
});

it("切换计费方式保留尚未保存的各项单价", () => {
  const rule = { base_micros: 120000 };
  const { rerender } = render(<PriceSetup unit="per_image" rule={rule} />);
  fireEvent.change(screen.getByLabelText("价格（元）"), {
    target: { value: "0.45" },
  });
  rerender(<PriceSetup unit="per_1k_tokens" rule={rule} />);
  fireEvent.change(screen.getByLabelText("每千输入 Token（元）"), {
    target: { value: "0.005" },
  });
  rerender(<PriceSetup unit="per_image" rule={rule} />);
  expect((screen.getByLabelText("价格（元）") as HTMLInputElement).value).toBe(
    "0.45",
  );
  rerender(<PriceSetup unit="per_1k_tokens" rule={rule} />);
  expect(
    (screen.getByLabelText("每千输入 Token（元）") as HTMLInputElement).value,
  ).toBe("0.005");
});

it("普通单价保留已有积分比例和倍率，不把政策字段当成必须编辑的JSON", () => {
  const rule = {
    base_micros: 120000,
    credit_ratio: "10",
    markup_multiplier: "1",
  };
  const { container } = render(
    <form>
      <input name="unit" value="per_image" readOnly />
      <PriceSetup unit="per_image" rule={rule} />
    </form>,
  );
  expect((screen.getByLabelText("价格（元）") as HTMLInputElement).value).toBe(
    "0.12",
  );
  fireEvent.change(screen.getByLabelText("价格（元）"), {
    target: { value: "0.25" },
  });
  expect(priceRule(new FormData(container.querySelector("form")!))).toEqual({
    ...rule,
    base_micros: 250000,
  });
});

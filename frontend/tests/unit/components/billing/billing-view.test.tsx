import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { BillingView } from "@/components/billing/billing-view";
afterEach(cleanup);

it("积分服务未开放不把原型数字或零余额作为账号事实", () => {
  render(<BillingView />);
  expect(screen.getAllByLabelText(/暂不可查询/)).toHaveLength(3);
  expect(screen.queryByText("120")).toBeNull();
  expect(screen.queryByText("0")).toBeNull();
  expect(
    screen.getByRole("link", { name: "充值积分" }).getAttribute("href"),
  ).toBe("/billing/recharge");
});

it.each(["membership", "recharge"] as const)(
  "%s 没有伪造可购买套餐或支付动作",
  (section) => {
    render(<BillingView section={section} />);
    expect(screen.queryByRole("button", { name: "暂未开放" })).toBeNull();
    expect(
      screen.getByRole("heading", {
        name: section === "membership" ? "会员计划即将上线" : "充值即将开放",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "返回积分总览" }).getAttribute("href"),
    ).toBe("/billing");
  },
);

it("shows authorized exact balances and keeps restricted credits separate", () => {
  render(
    <BillingView
      wallet={{
        enabled: true,
        payer: "team",
        general_credits: "12.34567",
        restricted_credits: "2.1",
        held_credits: "1.2",
        redeemed_today: "0.7",
      }}
    />,
  );
  expect(screen.getByText("12.34567")).toBeTruthy();
  expect(screen.getByText("1.2")).toBeTruthy();
  expect(screen.getByText(/限模型积分 2.1/)).toBeTruthy();
});

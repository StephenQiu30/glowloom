import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BillingView } from "@/components/billing/billing-view";
import BillingLayout from "@/app/billing/layout";
import { PageToolbarContext } from "@/components/layout/page-toolbar";

vi.mock("next/navigation", () => ({
  useSelectedLayoutSegment: () => "recharge",
}));
afterEach(cleanup);

it("积分服务未开放不把原型数字或零余额作为账号事实", () => {
  render(<BillingView />);
  expect(screen.getAllByLabelText(/暂不可查询/)).toHaveLength(3);
  expect(screen.queryByText("120")).toBeNull();
  expect(screen.queryByText("0")).toBeNull();
});

it("账务导航只挂载到共享顶栏，并标记当前路由", () => {
  const header = document.createElement("header");
  const { container } = render(
    <PageToolbarContext.Provider value={header}>
      <BillingLayout>
        <BillingView section="recharge" />
      </BillingLayout>
    </PageToolbarContext.Provider>,
  );
  expect(container.querySelector("nav")).toBeNull();
  expect(header.querySelectorAll("nav a")).toHaveLength(4);
  expect(
    header.querySelector('[aria-current="page"]')?.getAttribute("href"),
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

it("网关流水区分冻结、实扣、解冻与入账，并保留发生时间", () => {
  const events = [
    { kind: "grant", credits: "100" },
    { kind: "freeze", credits: "-20" },
    { kind: "settle", credits: "-8" },
    { kind: "release", credits: "12" },
    { kind: "redeem", credits: "0.00" },
  ];
  render(
    <BillingView
      section="transactions"
      wallet={{
        enabled: true,
        history_available: true,
        entries: events.map((entry) => ({
          ...entry,
          id: entry.kind,
          created_at: "2026-11-23T02:42:00Z",
          source: "workbench",
          model_key: "candidate-00000000-0000-4000-8000-000000000001",
        })),
      }}
    />,
  );
  for (const amount of [
    "+100 积分",
    "冻结 20 积分",
    "−8 积分",
    "返还 12 积分",
    "0 积分",
  ]) {
    expect(screen.getByText(amount)).toBeTruthy();
  }
  expect(screen.queryByText(/candidate-/)).toBeNull();
  expect(document.querySelector("time")?.textContent).toMatch(
    /11[-/]23.*\d{2}:42/,
  );
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import Home from "@/app/page";
import { AppProviders } from "@/app/providers";

describe("创作首页", () => {
  it("为未登录用户展示可执行的注册和登录入口", async () => {
    sessionStorage.clear();
    render(
      <AppProviders>
        <Home />
      </AppProviders>,
    );

    expect(
      await screen.findByRole("heading", {
        name: /把剧本，变成.*可追踪的成片。/,
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "导入剧本" })).toHaveAttribute(
      "href",
      "/register",
    );
    expect(screen.getByRole("link", { name: "继续制作" })).toHaveAttribute(
      "href",
      "/login",
    );
    expect(screen.queryByText("项目草稿已生成")).not.toBeInTheDocument();
  });
});

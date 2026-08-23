import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import ForbiddenPage from "@/app/forbidden/page";
import NotFound from "@/app/not-found";
import { SystemStatusPage } from "@/components/system/system-status-page";

describe("system status pages", () => {
  it("provides a clear recovery path for forbidden pages", () => {
    render(<ForbiddenPage />);

    expect(screen.getByRole("heading", { name: "无权访问此页面" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "返回首页" })).toHaveAttribute("href", "/");
  });

  it("provides a clear recovery path for missing pages", () => {
    render(<NotFound />);

    expect(screen.getByRole("heading", { name: "页面不存在" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看项目" })).toHaveAttribute("href", "/projects");
  });

  it("aligns login recovery with the shared borderless page baseline", () => {
    render(
      <SystemStatusPage
        description="登录后继续当前工作。"
        primaryAction={{ href: "/login", label: "前往登录" }}
        secondaryAction={{ href: "/", label: "返回首页" }}
        status="401"
        title="需要登录后继续"
      />,
    );

    const heading = screen.getByRole("heading", { name: "需要登录后继续" });
    expect(heading.closest("section")).toHaveClass("max-w-[1440px]");
    expect(screen.getByRole("banner", { name: "Lanverse 全局页眉" })).not.toHaveClass("border-b");
    expect(screen.getByRole("group", { name: "恢复访问操作" })).toBeInTheDocument();
  });
});

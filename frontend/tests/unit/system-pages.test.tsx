import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import ForbiddenPage from "@/app/forbidden/page";
import NotFound from "@/app/not-found";

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
});

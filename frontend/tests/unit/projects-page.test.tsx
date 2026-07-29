import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ProjectDashboard } from "@/app/projects/project-dashboard";

describe("projects workspace", () => {
  it("filters mock comic projects", async () => {
    const user = userEvent.setup();
    render(<ProjectDashboard />);

    expect(screen.getByRole("heading", { name: "项目库" })).toBeInTheDocument();
    expect(screen.getByText("Stephen 的创作空间")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开项目 她从画中来" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建项目" })).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "搜索项目" }), "长安");
    expect(screen.getByRole("link", { name: "打开项目 长安夜行录" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "打开项目 她从画中来" })).not.toBeInTheDocument();
  });
});

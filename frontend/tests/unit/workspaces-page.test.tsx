import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import WorkspacesPage from "@/app/workspaces/page";

describe("account and workspace settings", () => {
  it("renders and extends mock workspace data", async () => {
    const user = userEvent.setup();
    render(<WorkspacesPage />);

    expect(screen.getByRole("heading", { name: "账户与工作空间" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("Stephen")).toBeInTheDocument();
    expect(screen.getByText("Stephen 的创作空间")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建工作空间" })).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "空间名称" }), "青墨工作室");
    await user.click(screen.getByRole("button", { name: "创建工作空间" }));
    expect(screen.getByText("青墨工作室")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("工作空间“青墨工作室”已创建");
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ProjectWorkspace } from "@/app/projects/[projectId]/project-workspace";

describe("project production entry", () => {
  it("renders project overview and mock episode flow", async () => {
    const user = userEvent.setup();
    render(<ProjectWorkspace projectId="painting-girl" />);

    expect(screen.getByRole("heading", { name: "她从画中来" })).toBeInTheDocument();
    expect(screen.getByText("画中人")).toBeInTheDocument();
    expect(screen.getByText("确认 2 项角色资产变更")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "前往资产库" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建单集" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "创建单集" }));
    expect(screen.getByRole("status")).toHaveTextContent("第 09 集草稿已创建");
  });
});

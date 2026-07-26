import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ProjectsView } from "@/features/projects/projects-view";

describe("ProjectsView", () => {
  it("shows one primary action in the empty state and creates a trimmed title", async () => {
    const user = userEvent.setup();
    const create = vi.fn();
    render(<ProjectsView onCreate={create} projects={[]} />);

    expect(screen.getByText("还没有短剧项目")).toBeInTheDocument();
    const title = screen.getByRole("textbox", { name: "项目标题" });
    await user.type(title, "  雾中来信  ");
    await user.click(screen.getByRole("button", { name: "创建项目" }));

    expect(create).toHaveBeenCalledWith("雾中来信");
  });

  it("links each project to its episode Story workspace", () => {
    render(
      <ProjectsView
        onCreate={vi.fn()}
        projects={[
          {
            id: "project-1",
            title: "雾中来信",
            episodeId: "episode-1",
            hasConfirmedSource: false,
          },
        ]}
      />,
    );

    expect(screen.getByRole("link", { name: /打开雾中来信/ })).toHaveAttribute(
      "href",
      "/episodes/episode-1/story",
    );
    expect(screen.getByText("待输入来源")).toBeInTheDocument();
  });
});

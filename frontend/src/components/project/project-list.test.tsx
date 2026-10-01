import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { ProjectList, type ProjectListDisplayItem } from "./project-list";

afterEach(cleanup);

const projects: ProjectListDisplayItem[] = [
  {
    id: "active-project",
    name: "逆光",
    aspectRatio: "16:9",
    styleType: "stylized",
    status: "active",
    isDeleted: false,
  },
  {
    id: "archived-project",
    name: "旧城",
    aspectRatio: "9:16",
    styleType: "realistic",
    status: "archived",
    isDeleted: false,
  },
  {
    id: "deleted-project",
    name: "回收中的作品",
    aspectRatio: "16:9",
    styleType: "realistic",
    status: "archived",
    isDeleted: true,
  },
];

it("describes a genuinely empty supplied list without inventing projects or actions", () => {
  render(<ProjectList projects={[]} view="cards" onViewChange={vi.fn()} />);

  expect(screen.getByRole("status").textContent).toContain("当前列表没有项目");
  expect(screen.queryByRole("list")).toBeNull();
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.queryByRole("button", { name: /新建|删除|恢复/ })).toBeNull();
});

it("renders supplied cards and switches to a semantic table when the controlled view changes", () => {
  const onViewChange = vi.fn();
  const view = render(
    <ProjectList
      projects={projects}
      view="cards"
      onViewChange={onViewChange}
    />,
  );

  expect(screen.getByRole("list").querySelectorAll("li")).toHaveLength(3);
  expect(screen.getByRole("heading", { name: "逆光" })).toBeTruthy();
  expect(
    screen
      .getByRole("radio", { name: "卡片视图" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(screen.queryByRole("table")).toBeNull();

  fireEvent.click(screen.getByRole("radio", { name: "表格视图" }));
  expect(onViewChange).toHaveBeenCalledExactlyOnceWith("table");
  view.rerender(
    <ProjectList
      projects={projects}
      view="table"
      onViewChange={onViewChange}
    />,
  );

  const table = screen.getByRole("table", { name: "项目列表" });
  expect(
    within(table)
      .getAllByRole("columnheader")
      .map((cell) => cell.textContent),
  ).toEqual(["名称", "状态", "画幅", "风格"]);
  expect(within(table).getAllByRole("row")).toHaveLength(4);
  expect(screen.queryByRole("list")).toBeNull();
  expect(
    screen
      .getByRole("radio", { name: "表格视图" })
      .getAttribute("aria-checked"),
  ).toBe("true");
});

it("labels active, archived and recycled projects from supplied state", () => {
  render(
    <ProjectList projects={projects} view="table" onViewChange={vi.fn()} />,
  );

  const table = screen.getByRole("table");
  expect(
    within(within(table).getByRole("row", { name: /逆光/ })).getByText(
      "进行中",
    ),
  ).toBeTruthy();
  expect(
    within(within(table).getByRole("row", { name: /旧城/ })).getByText(
      "已归档",
    ),
  ).toBeTruthy();
  expect(
    within(within(table).getByRole("row", { name: /回收中的作品/ })).getByText(
      "回收中",
    ),
  ).toBeTruthy();
});

it("keeps long names intact and exposes native, focusable view buttons", () => {
  const longName = "一段很长的作品名称WithoutSpaces连续延伸直到容器边缘";
  const project = { ...projects[0], name: longName };
  const view = render(
    <ProjectList projects={[project]} view="cards" onViewChange={vi.fn()} />,
  );

  const heading = screen.getByRole("heading", { name: longName });
  expect(heading.textContent).toBe(longName);
  expect(heading.className).toContain("break-words");

  const tableButton = screen.getByRole("radio", { name: "表格视图" });
  expect(tableButton.tagName).toBe("BUTTON");
  expect(tableButton.getAttribute("type")).toBe("button");
  tableButton.focus();
  expect(document.activeElement).toBe(tableButton);

  view.rerender(
    <ProjectList projects={[project]} view="table" onViewChange={vi.fn()} />,
  );
  const nameCell = screen.getByRole("cell", { name: longName });
  expect(nameCell.textContent).toBe(longName);
  expect(nameCell.className).toContain("break-words");
});

it.each(["cards", "table"] as const)(
  "%s 菜单可复制完整项目和恢复复制任务，回收项目不提供复制",
  async (view) => {
    const onAction = vi.fn();
    render(
      <ProjectList
        projects={projects}
        view={view}
        onViewChange={vi.fn()}
        onAction={onAction}
      />,
    );
    const menu = screen.getByRole("button", { name: "逆光的项目操作" });
    menu.focus();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.click(
      await screen.findByRole("menuitem", { name: "复制完整项目" }),
    );
    expect(onAction).toHaveBeenLastCalledWith(projects[0], "copy");
    menu.focus();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.click(await screen.findByRole("menuitem", { name: "复制任务" }));
    expect(onAction).toHaveBeenLastCalledWith(projects[0], "copies");
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    const recycled = screen.getByRole("button", {
      name: "回收中的作品的项目操作",
    });
    recycled.focus();
    fireEvent.keyDown(recycled, { key: "ArrowDown" });
    await screen.findByRole("menuitem", { name: "恢复项目" });
    expect(screen.queryByRole("menuitem", { name: "复制完整项目" })).toBeNull();
  },
);
it("可写项目提供移动入口，归档与回收项目禁用或省略", async () => {
  const action = vi.fn();
  render(
    <ProjectList
      projects={projects}
      view="cards"
      onViewChange={vi.fn()}
      onAction={action}
    />,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "逆光的项目操作" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("menuitem", { name: "移动到目录" }));
  expect(action).toHaveBeenLastCalledWith(projects[0], "move");
  fireEvent.keyDown(screen.getByRole("button", { name: "旧城的项目操作" }), {
    key: "ArrowDown",
  });
  expect(
    (await screen.findByRole("menuitem", { name: "移动到目录" })).getAttribute(
      "aria-disabled",
    ),
  ).toBe("true");
});

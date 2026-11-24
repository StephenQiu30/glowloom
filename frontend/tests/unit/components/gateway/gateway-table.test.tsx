import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import {
  GatewayTable,
  GatewaySelectionSlot,
} from "@/components/gateway/gateway-table";
import { TooltipProvider } from "@/components/ui/tooltip";

afterEach(cleanup);
const rows = Array.from({ length: 22 }, (_, i) => ({
  id: String(i),
  label: `模型 ${i + 1}`,
  cells: [`模型 ${i + 1}`],
}));
function table(data = rows) {
  return (
    <TooltipProvider>
      <GatewayTable
        label="模型"
        columns={["模型"]}
        rows={data}
        empty="暂无模型"
      />
    </TooltipProvider>
  );
}

it("keeps a partial selection across pages and copies exactly the selected entries", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  const clipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  try {
    render(table());
    const checkbox = (name: string) => screen.getByRole("checkbox", { name });
    fireEvent.click(checkbox("选择 模型 2"));
    expect(checkbox("全选本页模型").getAttribute("aria-checked")).toBe("mixed");
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    fireEvent.click(checkbox("全选本页模型"));
    expect(screen.getByText("已选 3 项")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "复制所选条目" }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith("模型 2\n模型 21\n模型 22"),
    );
    fireEvent.click(screen.getByRole("button", { name: "上一页" }));
    expect(checkbox("选择 模型 2").getAttribute("aria-checked")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "清除选择" }));
    expect(checkbox("全选本页模型").getAttribute("aria-checked")).toBe("false");
  } finally {
    if (clipboard) Object.defineProperty(navigator, "clipboard", clipboard);
    else Reflect.deleteProperty(navigator, "clipboard");
  }
});

it("returns to an existing page and drops removed rows after refresh", () => {
  const view = render(table());
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择 模型 22" }));
  view.rerender(table(rows.slice(0, 2)));
  expect(screen.getByText("第 1 / 1 页")).toBeTruthy();
  expect(screen.queryByRole("group", { name: "批量操作" })).toBeNull();
  view.rerender(table([]));
  expect(screen.getByRole("button", { name: "下一页" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("checkbox", { name: "全选本页模型" })).toHaveProperty(
    "disabled",
    true,
  );
});

it("changes the page size without losing selected rows", async () => {
  render(table());
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择 模型 22" }));
  fireEvent.keyDown(
    screen.getByRole("combobox", { name: "模型分页每页数量" }),
    { key: "ArrowDown" },
  );
  expect(
    (await screen.findAllByRole("option")).map((item) => item.textContent),
  ).toEqual(["10 条", "20 条", "50 条"]);
  fireEvent.click(await screen.findByRole("option", { name: "50 条" }));
  expect(screen.getByText("第 1 / 1 页")).toBeTruthy();
  expect(screen.getByRole("checkbox", { name: "选择 模型 1" })).toBeTruthy();
  expect(
    screen
      .getByRole("checkbox", { name: "选择 模型 22" })
      .getAttribute("aria-checked"),
  ).toBe("true");
});

it("places bulk actions in the top toolbar and sends the confirmed IDs in a single request", async () => {
  const onDelete = vi
    .fn()
    .mockRejectedValueOnce(new Error("conflict"))
    .mockResolvedValue(undefined);
  const slot = document.createElement("div");
  document.body.prepend(slot);
  try {
    render(
      <TooltipProvider>
        <GatewaySelectionSlot.Provider value={slot}>
          <GatewayTable
            label="渠道"
            columns={["模型", "操作"]}
            empty="暂无渠道"
            rows={rows
              .slice(0, 2)
              .map((row) => ({
                ...row,
                deleteID: "route-" + row.id,
                cells: [row.label, null],
              }))}
            onDelete={onDelete}
          />
        </GatewaySelectionSlot.Provider>
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "删除渠道 模型 1" }));
    expect(
      screen.getByRole("alertdialog", { name: "删除渠道「模型 1」？" }),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(onDelete).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("checkbox", { name: "全选本页渠道" }));
    const bulk = screen.getByRole("button", { name: "删除所选渠道" });
    expect(slot.contains(bulk)).toBe(true);
    fireEvent.click(bulk);
    expect(
      screen.getByRole("alertdialog", { name: "删除选中的 2 个渠道？" }),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(onDelete).toHaveBeenCalledTimes(1);
    expect(onDelete).toHaveBeenCalledWith(["route-0", "route-1"]);
    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(slot.childElementCount).toBe(0);
    fireEvent.click(screen.getByRole("button", { name: "删除渠道 模型 2" }));
    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    await waitFor(() => expect(onDelete).toHaveBeenLastCalledWith(["route-1"]));
  } finally {
    slot.remove();
  }
});

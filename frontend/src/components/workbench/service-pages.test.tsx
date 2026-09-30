import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MediaPage } from "./asset-pages";
import { TaskDetailPage, TaskListPage } from "./task-pages";
import { AccountPage, AdminScreen } from "./admin-pages";

const { replace, search, pathname } = vi.hoisted(() => ({
  replace: vi.fn(),
  search: { value: "" },
  pathname: { value: "/media" },
}));

vi.mock("next/navigation", () => ({
  usePathname: () => pathname.value,
  useRouter: () => ({ replace }),
  useSearchParams: () => new URLSearchParams(search.value),
}));

afterEach(() => {
  cleanup();
  search.value = "";
  pathname.value = "/media";
  vi.clearAllMocks();
});

it("combines media type and trimmed search without changing the sample assets", () => {
  search.value = "type=video&q=%20晨雾%20";
  render(<MediaPage />);
  expect(screen.getByRole("heading", { name: "晨雾 · 视频预览" })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "港口 · 关键帧 A" })).toBeNull();
});

it("preserves media search when changing its type", () => {
  search.value = "q=港口&state=readonly";
  render(<MediaPage />);
  fireEvent.click(screen.getByRole("radio", { name: "视频" }));
  expect(replace).toHaveBeenCalledWith(
    "/media?q=%E6%B8%AF%E5%8F%A3&state=readonly&type=video",
    { scroll: false },
  );
  expect(
    (screen.getByRole("button", { name: "上传预览" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

it("keeps uploads as file-name previews and leaves the service action disabled", () => {
  render(<MediaPage />);
  fireEvent.click(screen.getByRole("button", { name: "上传预览" }));
  fireEvent.change(screen.getByLabelText("选择本地素材"), {
    target: { files: [new File(["sample"], "reference.png")] },
  });
  expect(screen.getByRole("status").textContent).toContain("reference.png");
  expect(
    (
      screen.getByRole("button", {
        name: "上传服务待接入",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});

it("does not substitute an image when the audio filter has no sample media", () => {
  search.value = "type=audio";
  render(<MediaPage />);
  expect(screen.getByRole("heading", { name: "没有匹配的素材" })).toBeTruthy();
  expect(screen.queryByRole("img")).toBeNull();
});

it("uses the all-tasks view for unsupported status query parameters", () => {
  pathname.value = "/tasks";
  search.value = "status=constructor";
  render(<TaskListPage />);
  expect(screen.getByRole("link", { name: /镜头 01 · 关键帧/ })).toBeTruthy();
  expect(screen.getByRole("link", { name: /第一集 · 配音批次/ })).toBeTruthy();
});

it("filters failed tasks and carries the query into status changes", () => {
  pathname.value = "/tasks";
  search.value = "status=failed&q=雾港";
  render(<TaskListPage />);
  expect(screen.getByRole("link", { name: /镜头 02 · 视频/ })).toBeTruthy();
  expect(screen.queryByRole("link", { name: /镜头 01 · 关键帧/ })).toBeNull();
  fireEvent.click(screen.getByRole("radio", { name: /已完成/ }));
  expect(replace).toHaveBeenCalledWith(
    "/tasks?status=succeeded&q=%E9%9B%BE%E6%B8%AF",
    { scroll: false },
  );
});

it("keeps an unknown submission unresolved without exposing a resubmit action", () => {
  render(<TaskDetailPage taskId="task-03" />);
  expect(
    screen.getByRole("heading", { name: "需要核对提交结果" }),
  ).toBeTruthy();
  expect(screen.getAllByText("费用待核对").length).toBeGreaterThan(0);
  expect(
    screen.queryByRole("button", { name: /重新提交|重新生成/ }),
  ).toBeNull();
  expect(
    (
      screen.getByRole("button", {
        name: "取消与核对服务待接入",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});

it("disables profile draft changes in readonly mode", () => {
  search.value = "state=readonly";
  render(<AccountPage />);
  expect((screen.getByLabelText("显示名称") as HTMLInputElement).disabled).toBe(
    true,
  );
  expect(
    (
      screen.getByRole("button", {
        name: "保存服务待接入",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});

it("filters provider connections without claiming they are connected", () => {
  search.value = "q=视频";
  render(<AdminScreen section="providers" />);
  expect(screen.getByRole("cell", { name: "视频供应商（样例）" })).toBeTruthy();
  expect(screen.queryByRole("cell", { name: "图像供应商（样例）" })).toBeNull();
  expect(screen.getByRole("cell", { name: "尚未接入" })).toBeTruthy();
});

it("keeps a model price change in a local unpublished preview", () => {
  render(<AdminScreen section="models" />);
  fireEvent.change(screen.getByLabelText("单位价格（元，样例）"), {
    target: { value: "2" },
  });
  expect(screen.getByText("¥10.00")).toBeTruthy();
  expect(screen.getByText(/草稿已修改/)).toBeTruthy();
  expect(
    (
      screen.getByRole("button", {
        name: "发布服务待接入",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});

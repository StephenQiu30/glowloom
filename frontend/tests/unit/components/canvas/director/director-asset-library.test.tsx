import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  DirectorAssetLibrary,
  type DirectorAssetLibraryProps,
} from "@/components/canvas/director/director-asset-library";
vi.mock("@/components/canvas/director/director-project-assets", () => ({
  DirectorProjectAssets: () => <p>项目素材列表</p>,
}));
vi.mock("@/components/canvas/director/director-asset-preview", () => ({
  DirectorAssetPreview: ({ name }: { name: string }) => (
    <span role="img" aria-label={name} />
  ),
}));
afterEach(cleanup);
function mount(
  tab: DirectorAssetLibraryProps["tab"] = "actors",
  disabled = false,
) {
  const props: DirectorAssetLibraryProps = {
    projectID: "project",
    tab,
    disabled,
    guard: vi.fn(),
    onActor: vi.fn(),
    onPrimitive: vi.fn(),
    onBuiltinModel: vi.fn(),
    onLocalImport: vi.fn(),
    onProjectPicker: vi.fn(),
    onProjectAssets: vi.fn(),
    onCanvasPicker: vi.fn(),
  };
  render(<DirectorAssetLibrary {...props} />);
  return props;
}
function tab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }), {
    button: 0,
    ctrlKey: false,
  });
}
it("shows real body previews, searches names and hides presets in Mine", () => {
  const props = mount();
  expect(screen.getAllByRole("img")).toHaveLength(12);
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "女性 · 儿童" },
  });
  expect(screen.getAllByRole("img")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: /女性 · 儿童/ }));
  expect(props.onActor).toHaveBeenCalledWith("female_child");
  tab("我的");
  expect(screen.queryAllByRole("img")).toHaveLength(0);
  expect(screen.getByText("项目素材列表")).toBeTruthy();
});
it("renders populated simplified-model categories, exact geometry set and metre heights", () => {
  const props = mount("objects");
  expect(screen.getAllByRole("img")).toHaveLength(45);
  tab("物体");
  expect(screen.getAllByRole("img")).toHaveLength(32);
  expect(screen.queryByText("项目素材列表")).toBeNull();
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "厨房" } });
  expect(screen.getAllByRole("img")).toHaveLength(4);
  fireEvent.click(screen.getByRole("button", { name: /冰箱/ }));
  expect(props.onBuiltinModel).toHaveBeenCalledWith(
    expect.objectContaining({ id: "kenney-fridge", heightM: 1.8 }),
  );
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "不存在的物品" },
  });
  expect(screen.getByText("没有匹配的简模物体")).toBeTruthy();
});
it("chooses background type before project or canvas source and keeps the type in callbacks", () => {
  const props = mount("backgrounds");
  fireEvent.click(screen.getByRole("button", { name: "添加背景" }));
  fireEvent.mouseDown(screen.getAllByRole("tab", { name: "全景背景" })[0], {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "从画布选择" }));
  expect(props.onCanvasPicker).toHaveBeenCalledWith("panorama");
});
it("disables scene-changing asset actions for read-only users", () => {
  const props = mount("actors", true);
  fireEvent.click(screen.getByRole("button", { name: /男性 · 标准/ }));
  fireEvent.click(screen.getByRole("button", { name: "导入本地角色" }));
  expect(props.onActor).not.toHaveBeenCalled();
  expect(props.onLocalImport).not.toHaveBeenCalled();
});

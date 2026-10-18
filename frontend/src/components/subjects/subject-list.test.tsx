import { render } from "../../../tests/feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SubjectList } from "./subject-list";
const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn() }));
vi.mock("@/api/bible", () => ({
  listBibleEntries: api.list,
  getBibleEntry: api.get,
}));
vi.mock("./subject-editor", () => ({ SubjectEditor: () => null }));
vi.mock("./subject-detail", () => ({ SubjectDetail: () => null }));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
const project = "11111111-1111-4111-8111-111111111111";
const id = "22222222-2222-4222-8222-222222222222";
const version = "33333333-3333-4333-8333-333333333333";
const detail = {
  head: {
    id,
    project_id: project,
    kind: "character",
    revision: 1,
    current_version_id: version,
  },
  current: {
    id: version,
    entry_id: id,
    project_id: project,
    character: { name: "角色" },
  },
};
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it.each(["success", "failure"])(
  "筛选切换后丢弃旧分页的 %s，不覆盖新游标或加载状态",
  async (outcome) => {
    let resolve!: (v: unknown) => void;
    let reject!: (e: unknown) => void;
    let deletedResolve!: (v: unknown) => void;
    api.get.mockResolvedValue(detail);
    api.list
      .mockResolvedValueOnce({ entries: [detail], next_cursor: "active-page" })
      .mockImplementationOnce(
        () =>
          new Promise((yes, no) => {
            resolve = yes;
            reject = no;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((yes) => {
            deletedResolve = yes;
          }),
      )
      .mockResolvedValue({ entries: [], next_cursor: undefined });
    render(<SubjectList projectID={project} />);
    await screen.findByText("角色");
    fireEvent.click(screen.getByRole("button", { name: "加载更多主体" }));
    await waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: "已删除" }));
    await waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
    await act(async () => {
      if (outcome === "success")
        resolve({ entries: [], next_cursor: "stale-page" });
      else reject(new Error("stale failure"));
    });
    expect(screen.getByLabelText("正在读取主体")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "加载更多主体" })).toBeNull();
    await act(async () =>
      deletedResolve({ entries: [], next_cursor: "deleted-page" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "加载更多主体" }));
    await waitFor(() => expect(api.list).toHaveBeenCalledTimes(4));
    expect(api.list.mock.calls[3][0]).toMatchObject({
      deleted: true,
      cursor: "deleted-page",
    });
    expect(api.list.mock.calls[1][1].signal.aborted).toBe(true);
  },
);

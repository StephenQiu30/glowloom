import { render } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectRenameDialog } from "@/components/projects/project-rename-dialog";
import { projectPresentationStore } from "@/components/projects/project-presentation-store";
import {
  normalizeProjectName,
  type Project,
} from "@/components/projects/project";
import { RequestError, setCreationScope } from "@/lib/request";
const fixture = vi.hoisted(() => ({
  project: vi.fn(),
  update: vi.fn(),
  close: vi.fn(),
  session: { actor: { id: "00000000-0000-4000-8000-000000000001" } },
  space: {
    selected: { id: "00000000-0000-4000-8000-000000000002", writable: true },
  },
}));
vi.mock("@/api/projects", () => ({
  getProject: fixture.project,
  updateProject: fixture.update,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => fixture.session,
  ensureSessionFresh: async () => fixture.session,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => fixture.space,
}));
const initial: Project = {
  id: "00000000-0000-4000-8000-000000000003",
  canvas_id: "00000000-0000-4000-8000-000000000004",
  name: "原名",
  revision: 1,
  aspect_ratio: "16:9",
  style_key: null,
  status: "active",
  is_delete: false,
  update_time: "2026-10-19T00:00:00Z",
};
function open() {
  return render(
    <ProjectRenameDialog project={initial} open onOpenChange={fixture.close} />,
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  fixture.project.mockReset().mockResolvedValue(initial);
  fixture.update.mockReset();
  projectPresentationStore.clear();
  setCreationScope(fixture.space.selected.id);
});
afterEach(cleanup);
it("matches Go whitespace and Unicode codepoint limits without normalization", () => {
  expect(normalizeProjectName("\u0085😀\u0085")).toBe("😀");
  expect(normalizeProjectName("\uFEFF名称\uFEFF")).toBe("\uFEFF名称\uFEFF");
  expect(normalizeProjectName("😀".repeat(50))).toHaveLength(100);
  expect(() => normalizeProjectName("😀".repeat(51))).toThrow();
  expect(() => normalizeProjectName("\u0000")).toThrow();
  expect(normalizeProjectName("e\u0301")).toBe("e\u0301");
});
it("reads current revision and retries an uncertain rename with the same frozen request", async () => {
  fixture.update
    .mockRejectedValueOnce(new Error("lost"))
    .mockResolvedValue({ ...initial, name: "新名", revision: 2 });
  open();
  const input = await screen.findByLabelText("画布名称");
  await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
  fireEvent.change(input, { target: { value: " 新名 " } });
  fireEvent.click(screen.getByRole("button", { name: "保存名称" }));
  await screen.findByRole("button", { name: "确认原改名" });
  fixture.project.mockResolvedValue({ ...initial, name: "新名", revision: 2 });
  fireEvent.click(screen.getByRole("button", { name: "确认原改名" }));
  await waitFor(() => expect(fixture.close).toHaveBeenCalledWith(false));
  expect(fixture.update).toHaveBeenCalledTimes(2);
  expect(fixture.update.mock.calls[1]).toEqual(fixture.update.mock.calls[0]);
});
it("a confirmed rename whose read failed can only reread, even after becoming readonly", async () => {
  fixture.update.mockResolvedValue({ ...initial, name: "新名", revision: 2 });
  fixture.project
    .mockResolvedValueOnce(initial)
    .mockResolvedValueOnce(initial)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue({
      ...initial,
      name: "新名",
      revision: 2,
      status: "archived",
    });
  open();
  const input = await screen.findByLabelText("画布名称");
  await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
  fireEvent.change(input, { target: { value: "新名" } });
  fireEvent.click(screen.getByRole("button", { name: "保存名称" }));
  await screen.findByText(/名称已保存/);
  fireEvent.click(screen.getByRole("button", { name: "重新读取最新名称" }));
  await waitFor(() => expect(fixture.close).toHaveBeenCalledWith(false));
  expect(fixture.update).toHaveBeenCalledTimes(1);
});
it("a definite first validation rejection permits correcting the input", async () => {
  fixture.update.mockRejectedValue(new RequestError("invalid_request", 422));
  open();
  const input = await screen.findByLabelText("画布名称");
  await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
  fireEvent.change(input, { target: { value: "新名" } });
  fireEvent.click(screen.getByRole("button", { name: "保存名称" }));
  await waitFor(() => expect(fixture.update).toHaveBeenCalledTimes(1));
  await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
  expect(screen.getByRole("button", { name: "保存名称" })).toBeTruthy();
});
it("a failed initial read offers a readonly retry and IME enter never submits", async () => {
  fixture.project
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue(initial);
  open();
  await screen.findByText(/暂时无法读取/);
  fireEvent.click(screen.getByRole("button", { name: "重新读取当前名称" }));
  const input = screen.getByLabelText("画布名称");
  await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
  fireEvent.compositionStart(input);
  fireEvent.keyDown(input, { key: "Enter", isComposing: true });
  expect(fixture.update).not.toHaveBeenCalled();
  fireEvent.compositionEnd(input);
});

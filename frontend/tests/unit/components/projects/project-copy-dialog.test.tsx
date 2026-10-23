import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectCopyDialog } from "@/components/projects/project-copy-dialog";
import { createProjectCopy } from "@/api/projectCopies";
import { RequestError } from "@/lib/request";
import type { Project } from "@/components/projects/project";
const proof = {
  actorId: "actor",
  scopeId: "scope",
  sessionGeneration: 1,
  scopeGeneration: 1,
};
vi.mock("@/components/projects/project-presentation", () => ({
  useProjectPresentation: () => ({ proof, namespace: "actor/scope" }),
}));
vi.mock("@/components/projects/project-presentation-store", () => ({
  isProjectProofCurrent: () => true,
  publishProjectInvalidation: vi.fn(),
}));
vi.mock("@/api/projectCopies", () => ({
  createProjectCopy: vi.fn(),
  getProjectCopy: vi.fn(),
}));
const project: Project = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "测试画布",
  revision: 7,
  canvas_id: null,
  aspect_ratio: "16:9",
  style_key: null,
  update_time: "2026-10-23T00:00:00Z",
};
beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
  vi.mocked(createProjectCopy).mockRejectedValue(
    new RequestError("dependency_unavailable", 503),
  );
});
afterEach(cleanup);
it("恢复未知复制结果时重用相同幂等键与源版本，不发送第二份意图", async () => {
  const view = render(
    <ProjectCopyDialog project={project} onClose={vi.fn()} />,
  );
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "复制画布" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "复制画布" }));
  await screen.findByRole("button", { name: "重新确认原操作" });
  await waitFor(() => expect(createProjectCopy).toHaveBeenCalledTimes(1));
  const first = vi.mocked(createProjectCopy).mock.calls[0];
  view.unmount();
  render(
    <ProjectCopyDialog
      project={{ ...project, revision: 99 }}
      onClose={vi.fn()}
    />,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "重新确认原操作" }),
  );
  await waitFor(() => expect(createProjectCopy).toHaveBeenCalledTimes(2));
  const second = vi.mocked(createProjectCopy).mock.calls[1];
  expect(second[1]).toEqual(first[1]);
  expect(second[2]?.headers).toEqual(first[2]?.headers);
  expect(second[1].expected_revision).toBe(7);
});
it("无法持久化恢复记录时不提交复制", async () => {
  const storage = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementation(() => {
      throw new Error("存储不可用");
    });
  render(<ProjectCopyDialog project={project} onClose={vi.fn()} />);
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "复制画布" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "复制画布" }));
  await screen.findByText("存储不可用");
  expect(createProjectCopy).not.toHaveBeenCalled();
  storage.mockRestore();
});

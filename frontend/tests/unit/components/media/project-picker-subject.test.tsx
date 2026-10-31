import { render } from "../../../feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectAssetDialog } from "@/components/media/project-asset-dialog";
const fixture = vi.hoisted(() => {
  const id = "11111111-1111-4111-8111-111111111111",
    version = "22222222-2222-4222-8222-222222222222",
    scope = "33333333-3333-4333-8333-333333333333";
  return {
    id,
    version,
    scope,
    read: vi.fn(),
    subject: {
      id,
      version_id: version,
      owner_scope_id: scope,
      revision: 3,
      name: "主演",
      description: "原版",
      primary: {
        binding_id: id,
        asset_id: version,
        asset_revision: 1,
        kind: "image",
        mime_type: "image/png",
        file_name: "主图",
      },
      references: [],
      favorite: false,
      created_at: "2026-10-28T00:00:00Z",
      updated_at: "2026-10-28T00:00:00Z",
    },
  };
});
vi.mock("@/api/subjects", () => ({ getSubjectVersion: fixture.read }));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: fixture.id } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ selected: { id: fixture.scope } }),
}));
vi.mock("@/components/media/project-picker-sources", () => ({
  ProjectPickerCanvases: () => null,
  ProjectPickerSubjects: ({ onSelect }: { onSelect: (s: unknown) => void }) => (
    <button onClick={() => onSelect(fixture.subject)}>选择主演</button>
  ),
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
vi.mock("@/components/subjects/subject-list", () => ({
  SubjectList: () => null,
}));
vi.mock("@/components/media/upload-dialog", () => ({
  UploadDialog: () => null,
}));
beforeEach(() => {
  fixture.read.mockReset();
});
afterEach(cleanup);
it("主体添加重新授权固定版本，拒绝新版替换与错误归属", async () => {
  const add = vi.fn();
  render(
    <ProjectAssetDialog
      projectID={fixture.id}
      initialTab="subjects"
      onAdd={vi.fn()}
      onUnifiedSubject={add}
      onClose={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "选择主演" }));
  fixture.read.mockResolvedValue({
    ...fixture.subject,
    version_id: fixture.id,
  });
  fireEvent.click(screen.getByRole("button", { name: "添加主体" }));
  await screen.findByRole("alert");
  expect(add).not.toHaveBeenCalled();
  expect(fixture.read.mock.calls[0][0]).toEqual({
    id: fixture.id,
    version: fixture.version,
  });
  fixture.read.mockResolvedValue(fixture.subject);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "添加主体" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "添加主体" }));
  await waitFor(() =>
    expect(add).toHaveBeenCalledExactlyOnceWith(fixture.subject),
  );
});
it("关闭主体选择后取消核验，迟到版本不会添加", async () => {
  let finish!: (v: unknown) => void;
  fixture.read.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const add = vi.fn();
  const view = render(
    <ProjectAssetDialog
      projectID={fixture.id}
      initialTab="subjects"
      onAdd={vi.fn()}
      onUnifiedSubject={add}
      onClose={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "选择主演" }));
  fireEvent.click(screen.getByRole("button", { name: "添加主体" }));
  const signal = fixture.read.mock.calls[0][1].signal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  await act(() => finish(fixture.subject));
  expect(add).not.toHaveBeenCalled();
});

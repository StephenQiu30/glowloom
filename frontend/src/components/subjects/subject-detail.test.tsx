import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SubjectDetail } from "./subject-detail";
import { RequestError } from "@/lib/request";
import type { Subject } from "./contracts";
const api = vi.hoisted(() => ({
  remove: vi.fn(),
  restore: vi.fn(),
  get: vi.fn(),
}));
vi.mock("@/api/bible", () => ({
  deleteBibleEntry: api.remove,
  restoreBibleEntry: api.restore,
  getBibleEntry: api.get,
  confirmBibleEntry: vi.fn(),
  listBibleVersions: vi.fn(),
}));
vi.mock("./subject-editor", () => ({ SubjectEditor: () => null }));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
const id = "22222222-2222-4222-8222-222222222222",
  project = "11111111-1111-4111-8111-111111111111",
  version = "33333333-3333-4333-8333-333333333333";
const subject = {
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
    number: 1,
    character: { name: "Subject", description: "" },
  },
} as Subject;
const proof = {
  sha256: "a".repeat(64),
  revision: 2,
  current_lines: 0,
  historical_lines: 0,
  aliases: 0,
};
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);
it("reviews actual zero references and submits the exact accepted proof", async () => {
  api.remove
    .mockRejectedValueOnce(
      new RequestError("impact_acknowledgment_required", 409, {
        impact: proof,
      }),
    )
    .mockResolvedValue({
      entry_id: id,
      kind: "character",
      revision: 2,
      version_id: version,
    });
  api.get.mockResolvedValue({
    ...subject,
    head: { ...subject.head, revision: 2, deleted: true },
  });
  const changed = vi.fn();
  render(
    <SubjectDetail subject={subject} onClose={vi.fn()} onChanged={changed} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  fireEvent.click(screen.getByRole("button", { name: "检查引用" }));
  await screen.findByText("已核对：没有关联的剧本引用。");
  expect(changed).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  await waitFor(() => expect(changed).toHaveBeenCalledTimes(1));
  expect(api.remove.mock.calls[1][1]).toEqual({
    expected_revision: 1,
    acknowledged_impact: proof,
  });
});
it("unknown deletion retries the same key and cannot dismiss the pending intent", async () => {
  api.remove
    .mockRejectedValueOnce(
      new RequestError("impact_acknowledgment_required", 409, {
        impact: proof,
      }),
    )
    .mockRejectedValueOnce(new RequestError("network_error"))
    .mockResolvedValue({
      entry_id: id,
      kind: "character",
      revision: 2,
      version_id: version,
    });
  api.get.mockResolvedValue({
    ...subject,
    head: { ...subject.head, revision: 2, deleted: true },
  });
  const changed = vi.fn();
  render(
    <SubjectDetail subject={subject} onClose={vi.fn()} onChanged={changed} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  fireEvent.click(screen.getByRole("button", { name: "检查引用" }));
  await screen.findByText("已核对：没有关联的剧本引用。");
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect(
    screen.getByRole("button", { name: "取消" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(changed).toHaveBeenCalledTimes(1));
  expect(api.remove.mock.calls[2]).toEqual(api.remove.mock.calls[1]);
});
it("restores the existing identity and requires the restored server state", async () => {
  api.restore.mockResolvedValue({
    entry_id: id,
    kind: "character",
    revision: 3,
    version_id: version,
  });
  api.get.mockResolvedValue({
    ...subject,
    head: { ...subject.head, revision: 3, deleted: false },
  });
  const changed = vi.fn();
  render(
    <SubjectDetail
      subject={{
        ...subject,
        head: { ...subject.head, revision: 2, deleted: true },
      }}
      onClose={vi.fn()}
      onChanged={changed}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "恢复主体" }));
  fireEvent.click(screen.getByRole("button", { name: "恢复主体" }));
  await waitFor(() => expect(changed).toHaveBeenCalledTimes(1));
  expect(api.restore.mock.calls[0][1]).toEqual({ expected_revision: 2 });
  expect(api.remove).not.toHaveBeenCalled();
});

it("blocks a stale revision until the subject is refreshed", async () => {
  api.remove.mockRejectedValue(new RequestError("revision_conflict", 409));
  api.get.mockResolvedValue({
    ...subject,
    head: { ...subject.head, revision: 2 },
  });
  render(
    <SubjectDetail subject={subject} onClose={vi.fn()} onChanged={vi.fn()} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  fireEvent.click(screen.getByRole("button", { name: "检查引用" }));
  await screen.findByText("主体已在其他窗口更新，请刷新后重新确认操作。");
  expect(
    screen.getByRole("button", { name: "检查引用" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "刷新主体后重试" }));
  await screen.findByRole("button", { name: "编辑主体" });
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  expect(
    screen.getByRole("button", { name: "检查引用" }).hasAttribute("disabled"),
  ).toBe(false);
});

it("accepts a committed receipt when a later restore already advanced the head", async () => {
  api.remove
    .mockRejectedValueOnce(
      new RequestError("impact_acknowledgment_required", 409, {
        impact: proof,
      }),
    )
    .mockResolvedValue({
      entry_id: id,
      kind: "character",
      revision: 2,
      version_id: version,
    });
  api.get.mockResolvedValue({
    ...subject,
    head: { ...subject.head, revision: 3, deleted: false },
  });
  const changed = vi.fn();
  render(
    <SubjectDetail subject={subject} onClose={vi.fn()} onChanged={changed} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  fireEvent.click(screen.getByRole("button", { name: "检查引用" }));
  await screen.findByText("已核对：没有关联的剧本引用。");
  fireEvent.click(screen.getByRole("button", { name: "删除主体" }));
  await screen.findByText(
    "操作已完成，主体随后又有更新。当前显示的是最新状态。",
  );
  expect(changed.mock.calls[0][0].head.deleted).toBe(false);
  expect(screen.queryByRole("button", { name: "重试原操作" })).toBeNull();
});

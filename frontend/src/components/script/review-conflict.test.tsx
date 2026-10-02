import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { ReviewConflict } from "./review-conflict";
import { getWorkspace } from "./source-queries";
import { getEpisodes } from "./review-queries";
import type { useReviewWriter } from "./use-review-writer";
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
}));
vi.mock("./review-queries", () => ({
  getEpisodes: vi.fn(),
  getStructure: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const intent = {
  ...scope,
  version: 1 as const,
  key: id(6),
  action: "confirm_split" as const,
  charCount: 9,
  body: {
    version_id: id(4),
    expected_revision: 7,
    expected_split_revision: 2,
    candidate_set_id: id(5),
    boundaries: [{ seq_no: 1, title: "原完整稿", span_start: 0, span_end: 9 }],
    ack_invalidate: false,
  },
};
const workspace = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
  state: {
    project_id: scope.projectId,
    org_id: scope.orgId,
    revision: 12,
    draft_version_id: id(4),
    updated_at: "2026-10-02T00:00:00Z",
  },
};
function control(
  error = new ApiError(409, "revision_conflict"),
): ReturnType<typeof useReviewWriter> {
  return {
    ready: true,
    intent: null,
    busy: false,
    error: error.message,
    storageError: undefined,
    rejected: { intent, cause: error },
    locked: true,
    submit: vi.fn(async () => {}),
    restoreStorage: vi.fn(async () => {}),
    replay: vi.fn(async () => {}),
    acknowledgeLatest: vi.fn(),
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getWorkspace).mockResolvedValue(workspace);
  vi.mocked(getEpisodes).mockResolvedValue({
    version_id: id(4),
    head: {
      version_id: id(4),
      split_revision: 5,
      candidate_split_set_id: id(8),
    },
    candidate: {
      id: id(8),
      org_id: scope.orgId,
      project_id: scope.projectId,
      version_id: id(4),
      kind: "candidate",
      origin: "rules",
      boundaries: [],
      created_at: workspace.state.updated_at,
    },
    episodes: [],
  });
});
afterEach(cleanup);
it("确定409读最新后仍需明确新提交，保原完整边界且更换CAS不偷确认影响", async () => {
  const writer = control();
  render(<ReviewConflict scope={scope} writer={writer} />);
  expect(writer.submit).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新审核事实并保留原稿" }),
  );
  const submit = await screen.findByRole("button", {
    name: "按最新修订明确另行提交原稿",
  });
  expect(writer.acknowledgeLatest).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(submit);
  await waitFor(() =>
    expect(writer.submit).toHaveBeenCalledWith({
      action: "confirm_split",
      charCount: 9,
      body: {
        ...intent.body,
        expected_revision: 12,
        expected_split_revision: 5,
        candidate_set_id: id(8),
      },
    }),
  );
});
it("非法真实影响DTO不能ack；scope变更不允许读后借新主体重提", async () => {
  const writer = control(
    new ApiError(409, "confirmation_required", undefined, {
      affected_episodes: [
        { episode_id: id(7), confirmed_structure_id: id(8), reason: "unknown" },
      ],
    }),
  );
  const ui = render(<ReviewConflict scope={scope} writer={writer} />);
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新审核事实并保留原稿" }),
  );
  expect(
    (
      await screen.findByRole("button", { name: "按最新修订明确另行提交原稿" })
    ).matches(":disabled"),
  ).toBe(true);
  expect(writer.submit).not.toHaveBeenCalled();
  ui.unmount();
  vi.mocked(getWorkspace).mockResolvedValue({
    ...workspace,
    current_actor_id: id(9),
  });
  render(<ReviewConflict scope={scope} writer={control()} />);
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新审核事实并保留原稿" }),
  );
  expect(await screen.findAllByRole("alert")).toBeTruthy();
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "按最新修订明确另行提交原稿" }),
    ).toBeNull(),
  );
});

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ScriptEpisodePanel } from "./script-episode-panel";
import * as review from "./review-queries";
import * as versions from "./version-queries";
import type { useReviewWriter } from "./use-review-writer";
vi.mock("./review-queries", async (original) => ({
  ...(await original<typeof import("./review-queries")>()),
  getEpisodes: vi.fn(),
  getStructure: vi.fn(),
}));
vi.mock("./version-queries", async (original) => ({
  ...(await original<typeof import("./version-queries")>()),
  findVersion: vi.fn(),
  versionSourceText: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const version = {
  id: id(4),
  version_no: 1,
  content_hash: "a".repeat(64),
  document_sha256: "b".repeat(64),
  source_manifest_sha256: "c".repeat(64),
  char_count: 9,
  source_count: 1,
  created_at: "2026-10-02T00:00:00Z",
};
const head = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
  state: {
    project_id: scope.projectId,
    org_id: scope.orgId,
    revision: 7,
    draft_version_id: version.id,
    updated_at: version.created_at,
  },
};
const episode = {
  id: id(7),
  org_id: scope.orgId,
  project_id: scope.projectId,
  script_version_id: version.id,
  split_set_id: id(6),
  seq_no: 1,
  title: "一",
  span_start: 0,
  span_end: 9,
  revision: 1,
  current_structure_id: id(8),
  inherit_status: "not_inherited",
  is_delete: false,
};
const view = {
  version_id: version.id,
  head: {
    version_id: version.id,
    split_revision: 2,
    candidate_split_set_id: id(5),
    confirmed_split_set_id: id(6),
  },
  candidate: {
    id: id(5),
    org_id: scope.orgId,
    project_id: scope.projectId,
    version_id: version.id,
    kind: "candidate" as const,
    origin: "sources" as const,
    boundaries: [{ seq_no: 1, title: "一", span_start: 0, span_end: 9 }],
    created_at: version.created_at,
  },
  episodes: [episode],
};
const document = {
  scenes: [
    {
      scene_key: id(9),
      seq_no: 1,
      heading: "原候选",
      location_text: "",
      time_of_day: "",
      span_start: 0,
      span_end: 9,
      items: [],
    },
  ],
  unassigned_lines: [],
};
const detail = {
  episode,
  structure: {
    id: id(8),
    org_id: scope.orgId,
    project_id: scope.projectId,
    episode_id: episode.id,
    version_no: 1,
    source_hash: version.content_hash,
    document,
    actor_id: scope.actorId,
    created_at: version.created_at,
  },
};
function writer(): ReturnType<typeof useReviewWriter> {
  return {
    ready: true,
    intent: null,
    busy: false,
    error: undefined,
    storageError: undefined,
    rejected: undefined,
    locked: false,
    submit: vi.fn(async () => {}),
    replay: vi.fn(async () => {}),
    restoreStorage: vi.fn(async () => {}),
    acknowledgeLatest: vi.fn(),
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(versions.findVersion).mockResolvedValue(version);
  vi.mocked(review.getEpisodes).mockResolvedValue(view);
  vi.mocked(review.getStructure).mockResolvedValue(detail);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount(control = writer()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ScriptEpisodePanel
        scope={scope}
        versionId={version.id}
        head={head}
        disabled={false}
        historical={false}
        writer={control}
        onBlockedChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
  return { client, control };
}
it("摘要与分集并行，正文片段懒读；完整候选冻结原CAS直到人工确认", async () => {
  const { control } = mount();
  expect(versions.findVersion).toHaveBeenCalled();
  expect(review.getEpisodes).toHaveBeenCalled();
  const edit = await screen.findByRole("button", {
    name: "编辑并确认完整分集",
  });
  expect(versions.versionSourceText).not.toHaveBeenCalled();
  fireEvent.click(edit);
  fireEvent.click(screen.getByRole("button", { name: "确认完整分集" }));
  await waitFor(() =>
    expect(control.submit).toHaveBeenCalledWith({
      action: "confirm_split",
      charCount: 9,
      body: {
        version_id: version.id,
        expected_revision: 7,
        expected_split_revision: 2,
        candidate_set_id: id(5),
        boundaries: view.candidate.boundaries,
        ack_invalidate: false,
      },
    }),
  );
});
it("确认弹窗冻结具体结构正文，后台刷新不能换成未审阅的新候选", async () => {
  const { client, control } = mount();
  const button = await screen.findByRole("button", {
    name: "人工确认当前结构",
  });
  await waitFor(() => expect(button.matches(":disabled")).toBe(false));
  fireEvent.click(button);
  const dialog = screen.getByRole("dialog", { name: "正式结构人工确认" });
  await act(async () => {
    client.setQueryData(
      [...review.reviewScopeKey(scope), "structure", episode.id, "current"],
      {
        ...detail,
        episode: { ...episode, current_structure_id: id(9), revision: 2 },
        structure: {
          ...detail.structure,
          id: id(9),
          version_no: 2,
          document: {
            ...document,
            scenes: [{ ...document.scenes[0], heading: "后来新候选" }],
          },
        },
      },
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  await waitFor(() =>
    expect(within(dialog).queryByText(/后来新候选/)).toBeNull(),
  );
  fireEvent.click(within(dialog).getByRole("checkbox"));
  fireEvent.click(
    within(dialog).getByRole("button", { name: "明确提交此审核" }),
  );
  expect(control.submit).toHaveBeenCalledWith({
    action: "confirm_structure",
    episodeId: episode.id,
    structureId: id(8),
    body: {
      expected_revision: 7,
      expected_episode_revision: 1,
      base_structure_version_no: 1,
      ack_invalidate: false,
    },
  });
});

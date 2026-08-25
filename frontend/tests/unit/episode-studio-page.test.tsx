import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/request")>()),
  default: requestMock,
}));

import { AppProviders } from "@/app/providers";
import { EpisodeProductionStudio } from "@/app/studio/[episodeId]/episode-production-studio";
import { setAccessToken } from "@/lib/auth-session";
import { ApiClientError } from "@/lib/request";

const projectId = "019fb2c0-a000-7000-8000-000000000002";
const episodeId = "019fb2c0-a000-7000-8000-000000000003";
const structureId = "019fb2c0-a000-7000-8000-000000000004";
const taskId = "019fb2c0-a000-7000-8000-000000000005";
const batchId = "019fb2c0-a000-7000-8000-000000000006";
const unitId = "019fb2c0-a000-7000-8000-000000000007";

const episode = {
  id: episodeId,
  project_id: projectId,
  name: "雨巷相逢",
  position: 1,
  target_duration_ms: 90_000,
  current_script_version_id: "019fb2c0-a000-7000-8000-000000000008",
};

const project = { id: projectId, name: "镜中长安", aspect_ratio: "9:16" };

function structure(status: "needs_review" | "confirmed", revision: number, taskStatus: string) {
  return {
    id: structureId,
    status,
    revision,
    script_version_id: episode.current_script_version_id,
    scenes: [
      {
        id: "019fb2c0-a000-7000-8000-000000000009",
        heading: "雨巷，夜",
        position: 1,
        narrative_units: [{ id: unitId, kind: "action", text: "顾清禾撑伞停下。" }],
        dialogues: [
          {
            id: "019fb2c0-a000-7000-8000-000000000010",
            speaker: "顾清禾",
            text: "你终于来了。",
          },
        ],
        tasks: [
          {
            id: taskId,
            kind: "shot_breakdown",
            label: "拆解场景分镜",
            status: taskStatus,
            required: true,
          },
        ],
      },
    ],
  };
}

const proposals = [
  {
    proposal_key: "shot-001",
    position: 1,
    title: "雨巷建立镜头",
    narrative_unit_version_ids: [unitId],
    spec: { duration_ms: 3000, visual: { shot_size: "wide", camera_movement: "static" } },
    risk_codes: [],
  },
  {
    proposal_key: "shot-002",
    position: 2,
    title: "顾清禾近景",
    narrative_unit_version_ids: [unitId],
    spec: { duration_ms: 2400, visual: { shot_size: "close_up", camera_movement: "push_in" } },
    risk_codes: [],
  },
];

function draft(status: "needs_review" | "approved" | "applied", revision: number, decisions: Record<string, string> = {}) {
  return {
    id: batchId,
    status,
    revision,
    candidate: { shots: proposals },
    decisions,
    result_hash: "a".repeat(64),
  };
}

type BackendState = {
  structure: ReturnType<typeof structure>;
  batch?: ReturnType<typeof draft>;
  shots?: Array<{ id: string; position: number; title: string; content_hash: string; spec: object }>;
};

function installBackend(initial: BackendState) {
  let currentStructure = initial.structure;
  let currentBatch = initial.batch;
  let currentShots = initial.shots ?? [];

  requestMock.mockImplementation(async (url: string, options?: { method?: string }) => {
    const method = options?.method ?? "GET";
    if (method === "GET" && url === `/api/v1/episodes/${episodeId}`) return { data: episode };
    if (method === "GET" && url === `/api/v1/projects/${projectId}`) return { data: project };
    if (method === "GET" && url === `/api/v1/projects/${projectId}/episodes`) return { data: [episode] };
    if (method === "GET" && url === `/api/v1/episodes/${episodeId}/structure`) return { data: currentStructure };
    if (method === "GET" && url === `/api/v1/episodes/${episodeId}/shots`) return { data: currentShots };
    if (method === "GET" && url === `/api/v1/episodes/${episodeId}/storyboard-draft`) {
      if (currentBatch) return { data: currentBatch };
      throw new ApiClientError("尚无候选", "not_found");
    }
    if (method === "GET" && url === `/api/v1/episodes/${episodeId}/storyboard-export`) {
      throw new ApiClientError("尚无导出", "not_found");
    }
    if (method === "GET" && url.includes("/api/v1/storyboard-exports/") && url.endsWith("/download")) {
      return new Blob(["zip-bytes"], { type: "application/zip" });
    }
    if (method === "POST" && url === `/api/v1/episode-structures/${structureId}/tasks/${taskId}/accept`) {
      currentStructure = structure("needs_review", 2, "accepted");
      return { data: currentStructure };
    }
    if (method === "POST" && url === `/api/v1/episode-structures/${structureId}/confirm`) {
      currentStructure = structure("confirmed", 3, "accepted");
      return { data: currentStructure };
    }
    if (method === "POST" && url === `/api/v1/storyboard-draft-batches/${batchId}/decisions`) {
      const accepted = Object.keys(currentBatch?.decisions ?? {}).length;
      currentBatch = draft("needs_review", 6 + accepted, {
        ...(currentBatch?.decisions ?? {}),
        [proposals[accepted].proposal_key]: "accepted",
      });
      return { data: currentBatch };
    }
    if (method === "POST" && url === `/api/v1/storyboard-draft-batches/${batchId}/approve`) {
      currentBatch = draft("approved", 8, {
        "shot-001": "accepted",
        "shot-002": "accepted",
      });
      return { data: currentBatch };
    }
    if (method === "POST" && url === `/api/v1/storyboard-draft-batches/${batchId}/apply-preflight`) {
      return { data: { batch_id: batchId, batch_revision: 8, order_hash: "b".repeat(64), impact_hash: "c".repeat(64), created: 2 } };
    }
    if (method === "POST" && url === `/api/v1/storyboard-draft-batches/${batchId}/apply`) {
      currentBatch = draft("applied", 9, {
        "shot-001": "accepted",
        "shot-002": "accepted",
      });
      currentShots = proposals.map((proposal, index) => ({
        id: `019fb2c0-a000-7000-8000-00000000002${index}`,
        position: proposal.position,
        title: proposal.title,
        content_hash: String(index + 1).repeat(64),
        spec: proposal.spec,
      }));
      return { data: { batch: currentBatch, shots: currentShots } };
    }
    if (method === "POST" && url === `/api/v1/episodes/${episodeId}/storyboard-exports/preflight`) {
      return { data: { order_hash: "d".repeat(64), allowed: true, shot_count: 2, blockers: [] } };
    }
    if (method === "POST" && url === `/api/v1/episodes/${episodeId}/storyboard-exports`) {
      return {
        data: {
          id: "019fb2c0-a000-7000-8000-000000000030",
          content_hash: "e".repeat(64),
          download_url: "/api/v1/storyboard-exports/019fb2c0-a000-7000-8000-000000000030/download",
          files: [{ name: "storyboard.json" }],
        },
      };
    }
    throw new Error(`未覆盖的请求：${method} ${url}`);
  });
}

describe("单集 MVP 生产工作台", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    requestMock.mockReset();
    Object.defineProperty(URL, "createObjectURL", { configurable: true, value: vi.fn(() => "blob:test") });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
  });

  it("接受场景任务后才能确认剧本结构", async () => {
    const user = userEvent.setup();
    installBackend({ structure: structure("needs_review", 1, "pending") });
    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "第 1 集 · 雨巷相逢" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "雨巷，夜 制作任务" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "确认剧本结构" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "接受" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "确认剧本结构" })).toBeEnabled());
    expect(requestMock).toHaveBeenCalledWith(
      `/api/v1/episode-structures/${structureId}/tasks/${taskId}/accept`,
      expect.objectContaining({ method: "POST", data: expect.objectContaining({ expected_revision: 1 }) }),
    );

    await user.click(screen.getByRole("button", { name: "确认剧本结构" }));
    expect(await screen.findByText("结构已确认")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(requestMock).toHaveBeenCalledWith(
      `/api/v1/episode-structures/${structureId}/confirm`,
      expect.objectContaining({ method: "POST", data: expect.objectContaining({ expected_revision: 2 }) }),
    );
  });

  it("把候选逐镜决议、批准、预检、原子写入并导出确定性分镜包", async () => {
    const user = userEvent.setup();
    installBackend({
      structure: structure("confirmed", 3, "accepted"),
      batch: draft("needs_review", 5),
    });
    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="storyboard" />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "第 1 集 · 雨巷相逢" })).toBeInTheDocument();
    expect(screen.getByText("雨巷建立镜头")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "批准整批草案" })).toBeDisabled();

    await user.click(screen.getAllByRole("button", { name: "接受此镜" })[0]);
    await user.click(screen.getByRole("button", { name: "接受此镜" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "批准整批草案" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "批准整批草案" }));

    await user.click(await screen.findByRole("button", { name: "预检写入影响" }));
    expect(await screen.findByText("将创建 2 个镜头")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "原子写入正式分镜" }));
    expect(await screen.findByRole("heading", { name: "2 个镜头" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "分镜准备度摘要" })).toHaveTextContent("顾清禾近景");

    await user.click(screen.getByRole("button", { name: "检查导出条件" }));
    expect(await screen.findByRole("region", { name: "分镜包预检结果" })).toHaveTextContent("允许导出 · 2 个镜头");
    await user.click(screen.getByRole("button", { name: "生成分镜包" }));
    const download = await screen.findByRole("button", { name: "下载分镜包" });
    await user.click(download);
    expect(requestMock).toHaveBeenCalledWith(
      "/api/v1/storyboard-exports/019fb2c0-a000-7000-8000-000000000030/download",
      expect.objectContaining({ responseType: "blob" }),
    );
    expect(await screen.findByText("分镜包下载已开始")).toBeInTheDocument();
    expect(screen.getByText(`SHA-256 ${"e".repeat(64)}`)).toBeInTheDocument();
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StoryboardExports } from "@/app/studio/[episodeId]/StoryboardExports";

const inputHash = "a".repeat(64);

function preflight(
  status: "ready" | "blocked" | "unavailable",
): API.ExportPreflightResponse {
  return {
    episode_id: "00000000-0000-0000-0000-000000000001",
    status,
    input_hash: status === "ready" ? inputHash : null,
    script_version_id:
      status === "ready" ? "00000000-0000-0000-0000-000000000002" : null,
    narrative_structure_id:
      status === "ready" ? "00000000-0000-0000-0000-000000000003" : null,
    narrative_unit_version_ids: [],
    shot_spec_version_ids: [],
    asset_version_ids: [],
    coverage_basis_hash: status === "ready" ? "b".repeat(64) : null,
    coverage_evaluation_hash: status === "ready" ? "c".repeat(64) : null,
    readiness_evaluation_hash: status === "ready" ? "d".repeat(64) : null,
    blockers:
      status === "ready"
        ? []
        : [
            {
              code: "COVERAGE_UNACCOUNTED",
              summary: "Required narrative units are not fully accounted for",
              next_action: "map_or_omit_narrative_units",
              shot_id: null,
              dependency_id: null,
            },
          ],
  };
}

describe("可信分镜包", () => {
  it("展示阻断事实并拒绝从 blocked 预检创建包", async () => {
    const onExport = vi.fn();
    render(
      <StoryboardExports
        busy={false}
        history={{ items: [], total: 0 }}
        preflight={preflight("blocked")}
        onDownload={vi.fn()}
        onExport={onExport}
        onPreflight={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "可信分镜包" })).toBeInTheDocument();
    expect(screen.getByText("COVERAGE_UNACCOUNTED")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "生成分镜包" })).toBeDisabled();
    expect(onExport).not.toHaveBeenCalled();
  });

  it("只用 ready 预检哈希创建并从成功历史受控下载", async () => {
    const user = userEvent.setup();
    const onExport = vi.fn().mockResolvedValue(undefined);
    const onDownload = vi.fn().mockResolvedValue(undefined);
    const mediaVersionId = "00000000-0000-0000-0000-000000000010";
    render(
      <StoryboardExports
        busy={false}
        history={{
          total: 1,
          items: [
            {
              id: "00000000-0000-0000-0000-000000000020",
              episode_id: "00000000-0000-0000-0000-000000000001",
              status: "succeeded",
              input_hash: inputHash,
              task_id: "00000000-0000-0000-0000-000000000030",
              error_code: null,
              manifest: {
                id: "00000000-0000-0000-0000-000000000040",
                schema_version: 1,
                input_hash: inputHash,
                script_version_id: "00000000-0000-0000-0000-000000000002",
                narrative_structure_id: "00000000-0000-0000-0000-000000000003",
                narrative_unit_version_ids: [],
                shot_spec_version_ids: [],
                asset_version_ids: [],
                coverage_basis_hash: "b".repeat(64),
                coverage_evaluation_hash: "c".repeat(64),
                files: [],
                media_version_id: mediaVersionId,
                package_sha256: "e".repeat(64),
                package_size_bytes: 2048,
                created_at: "2026-08-14T00:00:00Z",
              },
              created_at: "2026-08-14T00:00:00Z",
              updated_at: "2026-08-14T00:00:00Z",
            },
          ],
        }}
        preflight={preflight("ready")}
        onDownload={onDownload}
        onExport={onExport}
        onPreflight={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "生成分镜包" }));
    expect(onExport).toHaveBeenCalledWith(inputHash);

    await user.click(screen.getByRole("button", { name: "下载分镜包" }));
    expect(onDownload).toHaveBeenCalledWith(mediaVersionId);
    expect(screen.getByText(/2 KB/)).toBeInTheDocument();
  });
});

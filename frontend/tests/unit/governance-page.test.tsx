import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  createConsent: vi.fn(),
  getConsent: vi.fn(),
  listAuditEvents: vi.fn(),
  listConsents: vi.fn(),
  listMedia: vi.fn(),
  me: vi.fn(),
  reviseConsent: vi.fn(),
  revokeConsent: vi.fn(),
}));

vi.mock("@/api/identity", async () => {
  const actual = await vi.importActual<typeof import("@/api/identity")>(
    "@/api/identity",
  );
  return { ...actual, meApiV1MeGet: apiMocks.me };
});

vi.mock("@/api/governance", async () => {
  const actual = await vi.importActual<typeof import("@/api/governance")>(
    "@/api/governance",
  );
  return {
    ...actual,
    createConsentApiV1ConsentsPost: apiMocks.createConsent,
    getConsentApiV1ConsentsConsentIdGet: apiMocks.getConsent,
    listAuditEventsApiV1AuditEventsGet: apiMocks.listAuditEvents,
    listConsentsApiV1ConsentsGet: apiMocks.listConsents,
    reviseConsentApiV1ConsentsConsentIdRevisionsPost: apiMocks.reviseConsent,
    revokeConsentApiV1ConsentsConsentIdRevokePost: apiMocks.revokeConsent,
  };
});

vi.mock("@/api/media", async () => {
  const actual = await vi.importActual<typeof import("@/api/media")>("@/api/media");
  return { ...actual, listMediaApiV1MediaGet: apiMocks.listMedia };
});

import { AppProviders } from "@/app/providers";
import { GovernanceWorkspace } from "@/app/governance/governance-workspace";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb1e0-a00a-70f6-99dc-0b4e9e085565";
const subjectId = "019fb1e0-a043-73cd-8f84-781bef25b92a";
const proofId = "019fb1e0-a044-73cd-8f84-781bef25b92b";
const assetVersionId = "019fb1e0-a045-73cd-8f84-781bef25b92c";
const consentId = "019fb1e0-a052-7d45-9b43-6821b3b33440";
const now = "2026-07-30T08:00:00Z";

const auditEvents: API.AuditEventResponse[] = [
  {
    id: "019fb1e0-a060-7000-8000-000000000000",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "workspace.archived",
    target_type: "workspace",
    target_id: workspaceId,
    result: "succeeded",
    trace_id: "019fb1e0-a061-7000-8000-000000000000",
    metadata: {
      revision: 3,
      previous_status: "active",
      status: "archived",
    },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000001",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "project.updated",
    target_type: "project",
    target_id: "019fb1e0-a065-7000-8000-000000000001",
    result: "succeeded",
    trace_id: "019fb1e0-a066-7000-8000-000000000001",
    metadata: {
      revision: 4,
      changed_fields: ["description", "name"],
    },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000002",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "episode.reordered",
    target_type: "project",
    target_id: "019fb1e0-a065-7000-8000-000000000001",
    result: "succeeded",
    trace_id: "019fb1e0-a066-7000-8000-000000000002",
    metadata: {
      project_revision: 5,
      episode_count: 2,
    },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000003",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "script.version_published",
    target_type: "script_version",
    target_id: "019fb1e0-a070-7000-8000-000000000001",
    result: "succeeded",
    trace_id: "019fb1e0-a071-7000-8000-000000000001",
    metadata: {
      source_id: "019fb1e0-a072-7000-8000-000000000001",
      episode_id: "019fb1e0-a073-7000-8000-000000000001",
      version_no: 2,
      previous_version_id: null,
      current_version_id: "019fb1e0-a070-7000-8000-000000000001",
      episode_revision: 2,
    },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000004",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "asset.version_created",
    target_type: "asset_version",
    target_id: "019fb1e0-a080-7000-8000-000000000001",
    result: "succeeded",
    trace_id: "019fb1e0-a081-7000-8000-000000000001",
    metadata: {
      asset_id: "019fb1e0-a082-7000-8000-000000000001",
      asset_revision: 3,
      version_no: 3,
      kind: "character",
      set_as_current: true,
      previous_version_id: "019fb1e0-a083-7000-8000-000000000001",
      current_version_id: "019fb1e0-a080-7000-8000-000000000001",
    },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000005",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "shot.spec_version_created",
    target_type: "shot_spec_version",
    target_id: "019fb1e0-a090-7000-8000-000000000001",
    result: "succeeded",
    trace_id: "019fb1e0-a091-7000-8000-000000000001",
    metadata: {
      shot_id: "019fb1e0-a092-7000-8000-000000000001",
      episode_id: "019fb1e0-a093-7000-8000-000000000001",
      version_no: 4,
      shot_revision: 5,
      source: "asset_upgrade",
      previous_version_id: "019fb1e0-a094-7000-8000-000000000001",
      current_version_id: "019fb1e0-a090-7000-8000-000000000001",
    },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000006",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "consent.revoked",
    target_type: "consent",
    target_id: consentId,
    result: "succeeded",
    trace_id: "019fb1e0-a061-7000-8000-000000000001",
    metadata: { revision: 3, subject_type: "MEDIA_VERSION" },
    occurred_at: now,
  },
  {
    id: "019fb1e0-a060-7000-8000-000000000007",
    workspace_id: workspaceId,
    actor_id: "019fb1e0-a000-7000-8000-000000000001",
    action: "consent.registered",
    target_type: "consent",
    target_id: consentId,
    result: "succeeded",
    trace_id: "019fb1e0-a061-7000-8000-000000000002",
    metadata: { revision: 1, subject_type: "MEDIA_VERSION" },
    occurred_at: now,
  },
];

const scope: API.MediaUsageScope = {
  type: "media_usage",
  subject_type: "MEDIA_VERSION",
  subject_id: subjectId,
  rights_holder_role: "synthetic_creator",
  rights_types: ["copyright", "image", "voice"],
  authorized_purposes: ["ai_short_drama_generation", "public_distribution"],
  channels: ["lanverse_preview", "lanverse_download"],
  regions: ["CN"],
  valid_from: "2026-07-01T00:00:00Z",
  valid_to: "2027-07-30T23:59:59.999Z",
};

const revision: API.ConsentRevisionResponse = {
  id: "019fb1e0-a053-712b-ac68-2165c9f279b1",
  revision_no: 1,
  action: "register",
  scope,
  proof_media_version_ids: [proofId],
  reason: "角色形象与声音授权",
  created_by: "019fb1e0-a000-7000-8000-000000000001",
  created_at: now,
};

const detail: API.ConsentDetailResponse = {
  id: consentId,
  workspace_id: workspaceId,
  subject_identity: {
    reference: "synthetic-subject-adult-a",
    kind: "fictional_adult",
  },
  status: "active",
  revision: 1,
  current_revision_id: revision.id,
  current_revision: revision,
  revisions: [revision],
  created_by: revision.created_by,
  created_at: now,
  updated_at: now,
};

const media: API.MediaVersionResponse[] = [
  {
    id: subjectId,
    workspace_id: workspaceId,
    media_object_id: "019fb1e0-a040-7000-8000-000000000001",
    media_object_kind: "image",
    media_object_source_type: "upload",
    media_object_status: "active",
    media_object_current_version_id: subjectId,
    media_object_revision: 2,
    version_no: 2,
    filename: "character-reference.png",
    sha256: "a".repeat(64),
    size_bytes: 1024,
    mime_type: "image/png",
    probe_status: "ready",
    probe_attempt: 1,
    probe_error_code: null,
    probe_error_summary: null,
    probe_next_action: null,
    width: 1080,
    height: 1920,
    duration_ms: null,
    codec: null,
    container: "png",
    created_at: now,
  },
  {
    id: proofId,
    workspace_id: workspaceId,
    media_object_id: "019fb1e0-a041-7000-8000-000000000001",
    media_object_kind: "image",
    media_object_source_type: "upload",
    media_object_status: "active",
    media_object_current_version_id: proofId,
    media_object_revision: 1,
    version_no: 1,
    filename: "consent-proof.png",
    sha256: "b".repeat(64),
    size_bytes: 2048,
    mime_type: "image/png",
    probe_status: "ready",
    probe_attempt: 1,
    probe_error_code: null,
    probe_error_summary: null,
    probe_next_action: null,
    width: 1200,
    height: 1600,
    duration_ms: null,
    codec: null,
    container: "png",
    created_at: now,
  },
];

describe("governance consent workspace", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.clearAllMocks();
    setAccessToken("test-access-token");
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: revision.created_by,
          email: "creator@example.com",
          display_name: "创作者",
          avatar_url: null,
        },
        workspace: {
          id: workspaceId,
          name: "个人创作空间",
          status: "active",
          role: "owner",
          revision: 1,
        },
      },
    });
    apiMocks.listConsents.mockResolvedValue({
      data: { items: [detail], total: 1, limit: 50, offset: 0 },
    });
    apiMocks.listAuditEvents.mockResolvedValue({
      data: { items: auditEvents, total: 8, limit: 50, offset: 0 },
    });
    apiMocks.getConsent.mockResolvedValue({ data: detail });
    apiMocks.listMedia.mockResolvedValue({
      data: { items: media, total: 2, limit: 100, offset: 0 },
    });
    apiMocks.createConsent.mockResolvedValue({ data: detail });
    apiMocks.reviseConsent.mockResolvedValue({ data: detail });
    apiMocks.revokeConsent.mockResolvedValue({
      data: { ...detail, status: "revoked", revision: 2 },
    });
  });

  it("registers a typed consent from real media versions", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <GovernanceWorkspace />
      </AppProviders>,
    );

    expect(
      await screen.findByRole("heading", { name: "授权治理" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("synthetic-subject-adult-a")).toBeInTheDocument();
    expect(await screen.findByText("角色形象与声音授权")).toBeInTheDocument();
    expect(await screen.findByText(/2027年7月30日/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "新建授权" }));
    await user.clear(screen.getByLabelText("权利主体引用"));
    await user.type(screen.getByLabelText("权利主体引用"), "creator-character-a");
    await user.selectOptions(screen.getByLabelText("固定版本"), subjectId);
    await user.selectOptions(screen.getByLabelText("证明媒体"), proofId);
    await user.clear(screen.getByLabelText("登记说明"));
    await user.type(screen.getByLabelText("登记说明"), "角色形象授权确认");
    await user.click(screen.getByRole("button", { name: "登记授权" }));

    await waitFor(() => expect(apiMocks.createConsent).toHaveBeenCalledTimes(1));
    expect(apiMocks.createConsent).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_id: workspaceId,
        subject_identity: {
          reference: "creator-character-a",
          kind: "fictional_adult",
        },
        scope: expect.objectContaining({
          type: "media_usage",
          subject_type: "MEDIA_VERSION",
          subject_id: subjectId,
          regions: ["CN"],
        }),
        proof_media_version_ids: [proofId],
        idempotency_key: expect.any(String),
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("授权已登记");
  });

  it("lets the owner inspect and filter append-only audit events", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <GovernanceWorkspace />
      </AppProviders>,
    );

    const audit = await screen.findByRole("region", { name: "操作审计" });
    expect(await within(audit).findByText("授权撤销")).toBeInTheDocument();
    expect(within(audit).getByText("工作空间归档")).toBeInTheDocument();
    expect(within(audit).getByText("active → archived")).toBeInTheDocument();
    expect(within(audit).getByText("项目更新")).toBeInTheDocument();
    expect(within(audit).getByText("变更字段：description、name")).toBeInTheDocument();
    expect(within(audit).getByText("单集排序")).toBeInTheDocument();
    expect(within(audit).getByText("2 个单集")).toBeInTheDocument();
    expect(within(audit).getByText("剧本版本发布")).toBeInTheDocument();
    expect(within(audit).getByText("剧本 · v2")).toBeInTheDocument();
    expect(within(audit).getByText("资产版本创建")).toBeInTheDocument();
    expect(within(audit).getByText("character · v3")).toBeInTheDocument();
    expect(within(audit).getByText("分镜规格版本创建")).toBeInTheDocument();
    expect(within(audit).getByText("分镜规格 · v4")).toBeInTheDocument();
    expect(within(audit).getAllByText("revision 3")).toHaveLength(2);
    expect(within(audit).getAllByText(/MEDIA_VERSION/)).toHaveLength(2);

    await user.click(within(audit).getByRole("button", { name: "筛选" }));
    await user.selectOptions(within(audit).getByLabelText("动作"), "consent.revoked");
    await user.type(within(audit).getByLabelText("目标 UUID"), consentId);
    await user.click(
      within(audit).getByRole("button", { name: "应用审计筛选" }),
    );

    await waitFor(() => expect(apiMocks.listAuditEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({
        workspace_id: workspaceId,
        action: "consent.revoked",
        target_id: consentId,
        target_type: null,
      }),
    ));
  });

  it("prefills an asset-version consent from the readiness blocker handoff", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <GovernanceWorkspace
          prefill={{
            subjectType: "ASSET_VERSION",
            subjectId: assetVersionId,
            proofMediaVersionId: proofId,
          }}
        />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "登记新授权" })).toBeInTheDocument();
    expect(screen.getByLabelText("版本类型")).toHaveValue("ASSET_VERSION");
    expect(screen.getByLabelText("资产版本 UUID")).toHaveValue(assetVersionId);
    await waitFor(() =>
      expect(screen.getByLabelText("证明媒体")).toHaveValue(proofId),
    );

    await user.clear(screen.getByLabelText("权利主体引用"));
    await user.type(screen.getByLabelText("权利主体引用"), "creator-asset-version-a");
    await user.clear(screen.getByLabelText("登记说明"));
    await user.type(screen.getByLabelText("登记说明"), "资产版本生成授权确认");
    await user.click(screen.getByRole("button", { name: "登记授权" }));

    await waitFor(() => expect(apiMocks.createConsent).toHaveBeenCalledTimes(1));
    expect(apiMocks.createConsent).toHaveBeenCalledWith(
      expect.objectContaining({
        scope: expect.objectContaining({
          subject_type: "ASSET_VERSION",
          subject_id: assetVersionId,
        }),
        proof_media_version_ids: [proofId],
      }),
    );
  });

  it("revokes the selected consent with its current revision", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <GovernanceWorkspace />
      </AppProviders>,
    );

    await screen.findByText("角色形象与声音授权");
    await user.click(screen.getByRole("button", { name: "撤销授权" }));
    await user.type(screen.getByLabelText("撤销原因"), "权利人撤回授权");
    await user.click(screen.getByRole("button", { name: "确认撤销" }));

    await waitFor(() => expect(apiMocks.revokeConsent).toHaveBeenCalledTimes(1));
    expect(apiMocks.revokeConsent).toHaveBeenCalledWith(
      { consent_id: consentId },
      { expected_revision: 1, reason: "权利人撤回授权" },
    );
    expect(await screen.findByRole("status")).toHaveTextContent("授权已撤销");
  });

  it("appends a revision instead of replacing consent history", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <GovernanceWorkspace />
      </AppProviders>,
    );

    await screen.findByText("角色形象与声音授权");
    await user.click(screen.getByRole("button", { name: "修改范围" }));
    await user.clear(screen.getByLabelText("修订说明"));
    await user.type(screen.getByLabelText("修订说明"), "缩小授权渠道");
    await user.click(screen.getByRole("button", { name: "保存新修订" }));

    await waitFor(() => expect(apiMocks.reviseConsent).toHaveBeenCalledTimes(1));
    expect(apiMocks.reviseConsent).toHaveBeenCalledWith(
      { consent_id: consentId },
      expect.objectContaining({
        expected_revision: 1,
        reason: "缩小授权渠道",
        scope: expect.objectContaining({
          subject_type: "MEDIA_VERSION",
          subject_id: subjectId,
        }),
        proof_media_version_ids: [proofId],
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("新修订 r2 已保存");
  });
});

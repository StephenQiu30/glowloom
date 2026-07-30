import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  createConsent: vi.fn(),
  getConsent: vi.fn(),
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
const consentId = "019fb1e0-a052-7d45-9b43-6821b3b33440";
const now = "2026-07-30T08:00:00Z";

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
  valid_to: "2027-07-01T00:00:00Z",
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
    expect(screen.getByText("角色形象与声音授权")).toBeInTheDocument();

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
});

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UnifiedSubjectEditor } from "@/components/subjects/unified-editor";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
const api = vi.hoisted(() => ({ upload: vi.fn() }));
vi.mock("@/api/subjects", () => ({
  uploadSubjectSource: api.upload,
  createSubject: vi.fn(),
  getSubject: vi.fn(),
  updateSubject: vi.fn(),
  listSubjectVoiceOptions: vi.fn(),
  createSubjectVoiceJob: vi.fn(),
  getSubjectVoiceJob: vi.fn(),
  getSubjectVoiceCommand: vi.fn(),
}));
vi.mock("@/components/subjects/unified-media", () => ({
  UnifiedSubjectMedia: ({ label }: { label: string }) => <span>{label}</span>,
}));
const actorID = "11111111-1111-4111-8111-111111111111",
  scopeID = "22222222-2222-4222-8222-222222222222",
  asset = "33333333-3333-4333-8333-333333333333";
const existing: UnifiedSubject = {
  id: "44444444-4444-4444-8444-444444444444",
  owner_scope_id: scopeID,
  revision: 1,
  version_id: "55555555-5555-4555-8555-555555555555",
  name: "原主体",
  description: "原描述",
  primary: {
    binding_id: "66666666-6666-4666-8666-666666666666",
    asset_id: asset,
    asset_revision: 1,
    kind: "image",
    mime_type: "image/png",
    file_name: "原图.png",
  },
  references: [],
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
};
beforeEach(() => api.upload.mockReset());
afterEach(cleanup);
async function uploadUnknown(initial?: UnifiedSubject) {
  api.upload.mockRejectedValueOnce(new Error("network"));
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "保留的名字" },
  });
  const file = new File(["png"], "参考.png", { type: "image/png" });
  fireEvent.drop(document.querySelector(".unified-subject-main")!, {
    dataTransfer: { files: [file] },
  });
  await screen.findByRole("heading", { name: "上传参考" });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await screen.findByRole("button", { name: "重试原上传" });
  fireEvent.click(screen.getByRole("button", { name: "返回设置" }));
  return file;
}
it("真实设置与上传组合：挂起返回可编辑表单，继续确认保留File/key并完成采用", async () => {
  const file = await uploadUnknown();
  const resume = screen.getByRole("button", { name: "继续确认上传" });
  await waitFor(() => expect(document.activeElement).toBe(resume));
  expect(screen.getByLabelText("名称")).toHaveProperty("disabled", false);
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "保留的名字");
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    true,
  );
  api.upload.mockResolvedValue({
    asset: {
      id: asset,
      revision: 1,
      kind: "image",
      mime_type: "image/png",
      file_name: "参考.png",
    },
  });
  fireEvent.click(resume);
  expect(api.upload).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "重试原上传" }));
  await screen.findByText("主参考图");
  expect(screen.queryByRole("button", { name: "继续确认上传" })).toBeNull();
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    false,
  );
  expect(api.upload.mock.calls[1][1]).toBe(file);
  expect(api.upload.mock.calls[1][2].headers["Idempotency-Key"]).toBe(
    api.upload.mock.calls[0][2].headers["Idempotency-Key"],
  );
});
it("明确不采纳未知上传只放弃本表单意图，保留原主参考与名称", async () => {
  await uploadUnknown(existing);
  fireEvent.click(screen.getByRole("button", { name: "不采纳此文件" }));
  expect(screen.getByText("主参考图")).toBeTruthy();
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "保留的名字");
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    false,
  );
  expect(api.upload).toHaveBeenCalledOnce();
});

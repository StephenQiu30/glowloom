// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { pageBusinessMemory } from "@/lib/page-business-memory";
import { ConnectionLogoEditor } from "@/app/providers/_components/connection-logo-editor";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  upload: vi.fn(),
  save: vi.fn(),
  receipt: vi.fn(),
  session: "",
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ session_id: api.session, actor: { id: api.session } }),
}));
vi.mock("@/api/media", () => ({ uploadProviderLogo: api.upload }));
vi.mock("@/api/settings", () => ({
  updateProviderLogo: api.save,
  getConfigReceipt: api.receipt,
}));
vi.mock("@/components/providers/provider-logo", () => ({
  ProviderLogo: ({ logoAssetID }: { logoAssetID?: string }) => (
    <span data-testid="logo">{logoAssetID ?? "builtin"}</span>
  ),
}));
const provider: API.CreatedProvider = {
  id: "supplier",
  adapter_key: "openai",
  revision: 4,
  logo_asset_id: "old-logo",
};
beforeEach(() => {
  api.session = crypto.randomUUID();
  pageBusinessMemory.clear();
  api.upload.mockResolvedValue({ id: "new-logo" });
  api.save.mockResolvedValue({
    ...provider,
    revision: 5,
    logo_asset_id: "new-logo",
  });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  pageBusinessMemory.clear();
});
function open() {
  const saved = vi.fn();
  const view = render(
    <ConnectionLogoEditor provider={provider} onSaved={saved} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "编辑 Logo" }));
  return { saved, ...view };
}
function upload() {
  fireEvent.change(screen.getByLabelText("Logo（可选）"), {
    target: { files: [new File(["image"], "logo.png", { type: "image/png" })] },
  });
}
it("keeps the old supplier logo until save and cancellation restores it", async () => {
  open();
  upload();
  await waitFor(() =>
    expect(
      screen.getAllByTestId("logo").map((node) => node.textContent),
    ).toEqual(["old-logo", "new-logo"]),
  );
  expect(api.save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.getAllByTestId("logo").map((node) => node.textContent)).toEqual(
    ["old-logo"],
  );
  fireEvent.click(screen.getByRole("button", { name: "编辑 Logo" }));
  expect(screen.getAllByTestId("logo").map((node) => node.textContent)).toEqual(
    ["old-logo", "old-logo"],
  );
});
it("allows retry after upload failure without clearing the old logo", async () => {
  api.upload.mockRejectedValueOnce(new Error("upload failed"));
  open();
  upload();
  await screen.findByText("Logo 上传失败，请重试。");
  expect(screen.getAllByTestId("logo").map((node) => node.textContent)).toEqual(
    ["old-logo", "old-logo"],
  );
  fireEvent.click(screen.getByRole("button", { name: "重试上传" }));
  await waitFor(() =>
    expect(
      screen.getAllByTestId("logo").map((node) => node.textContent),
    ).toEqual(["old-logo", "new-logo"]),
  );
  expect(api.upload).toHaveBeenCalledTimes(2);
});
it("saves display metadata with its own expected revision", async () => {
  const { saved } = open();
  upload();
  await waitFor(() =>
    expect(screen.getAllByTestId("logo")[1]?.textContent).toBe("new-logo"),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存 Logo" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.save.mock.calls[0]).toEqual([
    { id: "supplier" },
    {
      request_id: expect.any(String),
      expected_revision: 4,
      logo_asset_id: "new-logo",
    },
  ]);
});
it("checks an unknown receipt after reopening before resending the same logo", async () => {
  api.save.mockRejectedValueOnce(new Error("response lost"));
  const first = open();
  upload();
  await waitFor(() =>
    expect(screen.getAllByTestId("logo")[1]?.textContent).toBe("new-logo"),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存 Logo" }));
  await screen.findByRole("button", { name: "核对并保存 Logo" });
  const input = api.save.mock.calls[0][1];
  first.unmount();
  const { saved } = open();
  api.receipt.mockResolvedValue({
    request_id: input.request_id,
    action: "provider.logo.set",
    target_id: "supplier",
    result: { ...provider, logo_asset_id: "new-logo", revision: 5 },
  });
  fireEvent.click(screen.getByRole("button", { name: "核对并保存 Logo" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.save).toHaveBeenCalledOnce();
});
it("resends the original logo body only after a receipt proves absence", async () => {
  api.save.mockRejectedValueOnce(new Error("response lost"));
  const first = open();
  upload();
  await waitFor(() =>
    expect(screen.getAllByTestId("logo")[1]?.textContent).toBe("new-logo"),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存 Logo" }));
  await screen.findByRole("button", { name: "核对并保存 Logo" });
  const input = api.save.mock.calls[0][1];
  first.unmount();
  open();
  api.receipt.mockRejectedValue(new RequestError("not_found", 404));
  fireEvent.click(screen.getByRole("button", { name: "核对并保存 Logo" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledTimes(2));
  expect(api.save.mock.calls[1][1]).toEqual(input);
});

it("clears the logo with null and accepts the omitted empty read projection", async () => {
  const { logo_asset_id: unusedLogo, ...withoutLogo } = provider;
  void unusedLogo;
  api.save.mockResolvedValue({ ...withoutLogo, revision: 5 });
  const { saved } = open();
  fireEvent.click(screen.getByRole("button", { name: "清除 Logo" }));
  fireEvent.click(screen.getByRole("button", { name: "保存 Logo" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.save.mock.calls[0][1].logo_asset_id).toBeNull();
});

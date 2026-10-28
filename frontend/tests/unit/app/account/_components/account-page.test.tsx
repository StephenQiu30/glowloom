import { SWRConfig } from "swr";
import { render } from "../../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Settings } from "@/app/account/_components/account-page";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  update: vi.fn(),
  clear: vi.fn(),
  theme: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => mocks }));
vi.mock("next-themes", () => ({ useTheme: () => ({ setTheme: mocks.theme }) }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => session,
  useSessionUpdate: () => mocks.update,
  clearPrivateSession: mocks.clear,
  revokeSession: vi.fn(),
}));
const profile = {
  id: "11111111-1111-4111-8111-111111111111",
  display_name: "Alice",
  email: "alice@example.com",
  revision: "1",
  platform_role: "creator",
};
const session = {
  actor: profile,
  session_id: "22222222-2222-4222-8222-222222222222",
};
let saved = false;
beforeEach(() => {
  saved = false;
  vi.clearAllMocks();
  URL.createObjectURL = vi.fn(() => "blob:avatar");
  URL.revokeObjectURL = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me") {
      if (options?.method === "PATCH") saved = true;
      return saved
        ? { ...profile, display_name: "Updated", revision: "2" }
        : profile;
    }
    if (url === "/api/me/preferences")
      return { revision: "1", theme: "system", prompt_suffix: "initial" };
    if (url === "/api/me/sessions")
      return {
        items: [
          {
            id: session.session_id,
            current: true,
            device_summary: "当前浏览器",
            last_active_at: new Date().toISOString(),
          },
        ],
        as_of: new Date().toISOString(),
      };
    if (url === "/api/me/security-events")
      return { items: [], next_cursor: null, as_of: new Date().toISOString() };
    return undefined;
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("profile saved state requires server readback and preserves explicit missing capabilities", async () => {
  render(<Settings initialView="profile" />);
  const name = await screen.findByLabelText("显示名");
  fireEvent.change(name, { target: { value: "Updated" } });
  fireEvent.click(screen.getByText("保存资料"));
  expect(await screen.findByText("个人资料已保存并读回。")).toBeTruthy();
  expect(mocks.update).toHaveBeenCalledWith(
    expect.objectContaining({
      actor: expect.objectContaining({
        display_name: "Updated",
        revision: "2",
      }),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "编辑资料" }));
  expect(screen.getByRole("region", { name: "编辑头像" })).toBeTruthy();
  expect(screen.queryByRole("region", { name: "更换邮箱" })).toBeNull();
  expect(screen.queryByRole("button", { name: "更换邮箱" })).toBeNull();
  expect(screen.getByLabelText("邮箱")).toHaveProperty("readOnly", true);
});
it("profile conflict preserves unsaved intent and offers reload", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (options?.method === "PATCH")
      throw new RequestError("revision_conflict", 409);
    return original(url, options);
  });
  render(<Settings initialView="profile" />);
  const name = await screen.findByLabelText("显示名");
  fireEvent.change(name, { target: { value: "Unsaved" } });
  fireEvent.click(screen.getByText("保存资料"));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(name).toHaveProperty("value", "Unsaved");
  expect(mocks.update).not.toHaveBeenCalled();
});
it("last administrator duties prevent showing confirmation flow", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) =>
    url === "/api/me/deletion/preview"
      ? {
          revision: "1",
          team_ids: [],
          blocking_duties: ["last_platform_administrator"],
          policy_version: "soft-delete-v1",
        }
      : original(url, options),
  );
  render(<Settings initialView="profile" />);
  await screen.findByLabelText("显示名");
  fireEvent.click(screen.getByLabelText("返回账号设置"));
  fireEvent.pointerDown(screen.getByLabelText("更多账号设置"), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByText("注销账号"));
  fireEvent.click(screen.getByText("检查注销影响"));
  expect(await screen.findByText("last_platform_administrator")).toBeTruthy();
  expect(screen.queryByText("验证密码并发送验证码")).toBeNull();
  expect(mocks.clear).not.toHaveBeenCalled();
});
it("revocation refreshes directory and security activity contains no private payload", async () => {
  render(<Settings initialView="profile" />);
  await screen.findByLabelText("显示名");
  fireEvent.click(screen.getByLabelText("返回账号设置"));
  fireEvent.pointerDown(screen.getByLabelText("更多账号设置"), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByText("密码与会话"));
  fireEvent.click(screen.getByText("退出其他设备"));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/me/sessions/revoke-others",
      expect.objectContaining({ method: "POST" }),
    ),
  );
  expect(
    await screen.findByRole("heading", { name: "暂无安全活动" }),
  ).toBeTruthy();
});

it("security event failure cannot block loaded profile or leave loading visible", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/security-events")
      throw new RequestError("temporarily_unavailable", 503);
    return original(url, options);
  });
  render(<Settings initialView="profile" />);
  expect(await screen.findByLabelText("显示名")).toHaveProperty(
    "value",
    "Alice",
  );
  expect(screen.queryByText("正在加载账号资料…")).toBeNull();
  fireEvent.click(screen.getByLabelText("返回账号设置"));
  fireEvent.pointerDown(screen.getByLabelText("更多账号设置"), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByText("密码与会话"));
  expect(
    await screen.findByRole("heading", { name: "安全活动暂时无法读取" }),
  ).toBeTruthy();
});

it("清除头像经生成客户端传送 HTTP null 并以本人读回结果更新绑定", async () => {
  let removed = false;
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/avatar")
      return {
        url: "https://example.com/avatar",
        expires_at: new Date(Date.now() + 60000).toISOString(),
      };
    if (url === "/api/me") {
      if (options?.method === "PATCH") {
        expect(options.data).toMatchObject({
          avatar_asset_id: null,
          expected_revision: "1",
        });
        removed = true;
      }
      return {
        ...profile,
        avatar_asset_id: removed ? "" : "old-asset",
        revision: removed ? "2" : "1",
      };
    }
    return original(url, options);
  });
  render(<Settings initialView="profile" />);
  fireEvent.click(await screen.findByText("恢复默认头像"));
  expect(removed).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  await screen.findByText("个人资料已保存并读回。");
  expect(screen.queryByText("恢复默认头像")).toBeNull();
  expect(mocks.update).toHaveBeenCalledWith(
    expect.objectContaining({
      actor: expect.objectContaining({ avatar_asset_id: "", revision: "2" }),
    }),
  );
});

it("profile read error stays in the dialog and retry replaces it with loading", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  let release: ((value: unknown) => void) | undefined;
  let attempts = 0;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me") {
      attempts += 1;
      if (attempts === 1) throw new RequestError("dependency_unavailable", 503);
      return new Promise((resolve) => {
        release = resolve;
      });
    }
    return original(url, options);
  });
  render(<Settings initialView="profile" />);
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    expect.stringContaining("个人资料暂时无法读取"),
  );
  expect(screen.queryByText("dependency_unavailable")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    expect.stringContaining("正在读取个人资料"),
  );
  expect(screen.queryByRole("alert")).toBeNull();
  await waitFor(() => expect(release).toBeTypeOf("function"));
  release!(profile);
  expect(await screen.findByLabelText("显示名")).toHaveProperty(
    "value",
    "Alice",
  );
});

it("theme is applied only after a matching service readback", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  let committed = false;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/preferences") {
      if (options?.method === "PATCH") committed = true;
      return {
        revision: committed ? "2" : "1",
        theme: committed ? "dark" : "system",
        prompt_suffix: "initial",
      };
    }
    return original(url, options);
  });
  render(<Settings />);
  fireEvent.click(
    await screen.findByRole("button", { name: /主题模式.*跟随系统/ }),
  );
  fireEvent.click(screen.getByRole("radio", { name: "深色" }));
  expect(
    await screen.findByRole("button", { name: /主题模式.*深色/ }),
  ).toBeTruthy();
  expect(mocks.theme).toHaveBeenLastCalledWith("dark");
  expect(
    vi
      .mocked(request)
      .mock.calls.some(
        ([url, options]) =>
          url === "/api/me/preferences" && options?.method === "PATCH",
      ),
  ).toBe(true);
});
it("a mismatched preference readback keeps the choice open and does not announce success", async () => {
  render(<Settings />);
  fireEvent.click(
    await screen.findByRole("button", { name: /主题模式.*跟随系统/ }),
  );
  fireEvent.click(screen.getByRole("radio", { name: "深色" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(
    screen
      .getByRole("radio", { name: "跟随系统" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(mocks.theme).not.toHaveBeenCalledWith("dark");
  expect(screen.queryByText("偏好已保存。")).toBeNull();
});

it("saves biography then background using the new revision in a scoped cache", async () => {
  let revision = 1,
    bio = "",
    background: "dots" | "lines" | "blank" = "dots";
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me" || url === "/api/me/preferences") {
      const body = options?.data as Record<string, string> | undefined;
      if (options?.method === "PATCH") {
        if (body?.expected_revision !== String(revision))
          throw new RequestError("revision_conflict", 409);
        revision++;
        if (body?.bio !== undefined) bio = body.bio;
        if (body?.canvas_background)
          background = body.canvas_background as typeof background;
      }
      return url === "/api/me"
        ? { ...profile, bio, revision: String(revision) }
        : {
            revision: String(revision),
            canvas_background: background,
            theme: "dark",
            prompt_suffix: "",
          };
    }
    return { items: [] };
  });
  render(<Settings initialView="profile" />, {
    wrapper: ({ children }) => (
      <SWRConfig value={{ provider: () => new Map() }}>{children}</SWRConfig>
    ),
  });
  fireEvent.change(await screen.findByLabelText("个人简介"), {
    target: { value: "光影创作者" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  fireEvent.click(
    await screen.findByRole("button", { name: /画布背景.*点阵/ }),
  );
  fireEvent.click(screen.getByRole("radio", { name: "网格" }));
  expect(
    await screen.findByRole("button", { name: /画布背景.*网格/ }),
  ).toBeTruthy();
  expect(bio).toBe("光影创作者");
  expect(screen.queryByRole("alert")).toBeNull();
});

it("embedded profile cancel returns to the current personal-space tab without saving", async () => {
  const onClose = vi.fn();
  render(
    <Dialog open>
      <DialogContent>
        <Settings embedded initialView="profile" onClose={onClose} />
      </DialogContent>
    </Dialog>,
  );
  fireEvent.change(await screen.findByLabelText("显示名"), {
    target: { value: "Discarded" },
  });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(onClose).toHaveBeenCalledOnce();
  expect(saved).toBe(false);
});
it("embedded profile save closes only after the server readback updates the session", async () => {
  const onClose = vi.fn(() => expect(mocks.update).toHaveBeenCalled());
  render(
    <Dialog open>
      <DialogContent>
        <Settings embedded initialView="profile" onClose={onClose} />
      </DialogContent>
    </Dialog>,
  );
  fireEvent.change(await screen.findByLabelText("显示名"), {
    target: { value: "Updated" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  expect(saved).toBe(true);
});

it("选非正方形头像后直接保存资料会上传原文件、绑定并读回更新会话", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  let bound = false;
  const file = new File(["wide-image"], "landscape.png", { type: "image/png" });
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/avatar/uploads") {
      expect(options?.data).toBeInstanceOf(FormData);
      expect((options?.data as FormData).get("file")).toBe(file);
      return { id: "new-asset" };
    }
    if (url === "/api/me") {
      if (options?.method === "PATCH") {
        expect(options.data).toMatchObject({
          avatar_asset_id: "new-asset",
          expected_revision: "1",
          display_name: "New name",
          bio: "New bio",
        });
        expect(options.data).not.toHaveProperty("email");
        bound = true;
      }
      return {
        ...profile,
        avatar_asset_id: bound ? "new-asset" : "",
        revision: bound ? "2" : "1",
      };
    }
    return original(url, options);
  });
  render(<Settings initialView="profile" />);
  fireEvent.change(await screen.findByLabelText("显示名"), {
    target: { value: "New name" },
  });
  fireEvent.change(screen.getByLabelText("个人简介"), {
    target: { value: "New bio" },
  });
  fireEvent.change(screen.getByLabelText(/选择头像/), {
    target: { files: [file] },
  });
  expect(screen.queryByRole("checkbox", { name: /裁剪/ })).toBeNull();
  expect(screen.queryByRole("button", { name: "上传并保存头像" })).toBeNull();
  expect(bound).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  await screen.findByText("个人资料已保存并读回。");
  expect(mocks.update).toHaveBeenCalledWith(
    expect.objectContaining({
      actor: expect.objectContaining({
        avatar_asset_id: "new-asset",
        revision: "2",
      }),
    }),
  );
});

it("头像上传失败保留选图和资料，重试成功后才关闭弹窗", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  const onClose = vi.fn();
  let uploads = 0;
  let patched = false;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/avatar/uploads") {
      if (++uploads === 1)
        throw new RequestError("temporarily_unavailable", 503);
      return { id: "new-asset" };
    }
    if (url === "/api/me") {
      if (options?.method === "PATCH") patched = true;
      return {
        ...profile,
        avatar_asset_id: patched ? "new-asset" : "",
        revision: patched ? "2" : "1",
      };
    }
    return original(url, options);
  });
  render(
    <Dialog open>
      <DialogContent>
        <Settings embedded initialView="profile" onClose={onClose} />
      </DialogContent>
    </Dialog>,
  );
  fireEvent.change(await screen.findByLabelText("显示名"), {
    target: { value: "Keep draft" },
  });
  fireEvent.change(screen.getByLabelText(/选择头像/), {
    target: { files: [new File(["pixels"], "a.webp", { type: "image/webp" })] },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  await screen.findByRole("alert");
  expect(patched).toBe(false);
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByLabelText("显示名")).toHaveProperty("value", "Keep draft");
  expect(screen.getByText("撤销头像更改")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  expect(uploads).toBe(2);
});

it("头像绑定读回不一致不宣布成功并保留弹窗", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  let patched = false;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/avatar/uploads") return { id: "new-asset" };
    if (url === "/api/me") {
      if (options?.method === "PATCH") patched = true;
      return {
        ...profile,
        revision: patched ? "2" : "1",
        avatar_asset_id: "old-asset",
      };
    }
    if (url === "/api/me/avatar") return { url: "https://example.com/avatar" };
    return original(url, options);
  });
  const onClose = vi.fn();
  render(
    <Dialog open>
      <DialogContent>
        <Settings embedded initialView="profile" onClose={onClose} />
      </DialogContent>
    </Dialog>,
  );
  fireEvent.change(await screen.findByLabelText(/选择头像/), {
    target: { files: [new File(["pixels"], "a.png", { type: "image/png" })] },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存资料" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByText("个人资料已保存并读回。")).toBeNull();
  expect(onClose).not.toHaveBeenCalled();
  expect(mocks.update).not.toHaveBeenCalled();
});

it("取消选中的头像不上传，再次编辑不保留头像草稿", async () => {
  render(<Settings initialView="profile" />);
  fireEvent.change(await screen.findByLabelText(/选择头像/), {
    target: { files: [new File(["pixels"], "a.png", { type: "image/png" })] },
  });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  fireEvent.click(screen.getByRole("button", { name: "编辑资料" }));
  expect(screen.queryByText("撤销头像更改")).toBeNull();
  expect(
    vi
      .mocked(request)
      .mock.calls.some(([url]) => url === "/api/me/avatar/uploads"),
  ).toBe(false);
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:avatar");
});

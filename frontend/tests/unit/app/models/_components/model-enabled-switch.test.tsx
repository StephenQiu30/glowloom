import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelRowEnabledSwitch } from "@/app/models/_components/model-enabled-switch";

const change = vi.hoisted(() => vi.fn());
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "admin" } }),
}));
vi.mock("@/app/models/_components/model-activation", () => ({
  changeModelActivation: change,
}));
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

it("已启用但不可用时开关仍开启，点击停用期间保留旧值且阻止重复提交", async () => {
  let finish!: () => void;
  change.mockReturnValue(
    new Promise<void>((resolve) => {
      finish = resolve;
    }),
  );
  const saved = vi.fn();
  render(
    <ModelRowEnabledSwitch
      model={{
        id: "model",
        status: "active",
        available: false,
        display_name: "视频模型",
      }}
      onSaved={saved}
      disabled={false}
    />,
  );
  const toggle = screen.getByRole<HTMLButtonElement>("switch", {
    name: "启用 视频模型",
  });
  expect(toggle.getAttribute("aria-checked")).toBe("true");
  fireEvent.click(toggle);
  fireEvent.click(toggle);
  expect(change).toHaveBeenCalledOnce();
  expect(change.mock.calls[0][1]).toBe("disable");
  expect(toggle.disabled).toBe(true);
  expect(toggle.getAttribute("aria-checked")).toBe("true");
  finish();
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
});

it("启用失败保留关闭状态，显示反馈且重新读取服务端", async () => {
  change.mockRejectedValue(new Error("请先保存有效价格"));
  const saved = vi.fn();
  render(
    <ModelRowEnabledSwitch
      model={{ id: "model", status: "disabled", display_name: "待配置模型" }}
      onSaved={saved}
      disabled={false}
    />,
  );
  const toggle = screen.getByRole<HTMLButtonElement>("switch", {
    name: "启用 待配置模型",
  });
  fireEvent.click(toggle);
  await screen.findByText("请先保存有效价格");
  await waitFor(() => expect(toggle.disabled).toBe(false));
  expect(toggle.getAttribute("aria-checked")).toBe("false");
  expect(saved).toHaveBeenCalledOnce();
});

it("归档模型不能通过启用开关绕过恢复流程", () => {
  render(
    <ModelRowEnabledSwitch
      model={{ id: "model", status: "archived" }}
      onSaved={vi.fn()}
      disabled={false}
    />,
  );
  const toggle = screen.getByRole<HTMLButtonElement>("switch");
  expect(toggle.disabled).toBe(true);
  fireEvent.click(toggle);
  expect(change).not.toHaveBeenCalled();
});

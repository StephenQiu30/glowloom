import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { CreateProjectDialog } from "./create-project-dialog";
import type { CreationBody } from "./creation";

afterEach(cleanup);
function setup(
  onSubmit = vi
    .fn<(body: CreationBody, key: string) => Promise<{ id: string }>>()
    .mockResolvedValue({ id: "584ad191-2932-4d7c-bccb-0b9d481c5a76" }),
) {
  const onCreated = vi.fn();
  render(
    <CreateProjectDialog
      onSubmit={onSubmit}
      onCreated={onCreated}
      presets={[]}
      presetsPending={false}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "新建项目" }));
  return { onSubmit, onCreated };
}
it("提交真实创建端口并仅在成功后打开项目，不用假预设", async () => {
  const { onSubmit, onCreated } = setup();
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "逆光" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await waitFor(() =>
    expect(onCreated).toHaveBeenCalledExactlyOnceWith(
      "584ad191-2932-4d7c-bccb-0b9d481c5a76",
    ),
  );
  expect(onSubmit.mock.calls[0]?.[0]).toEqual({
    name: "逆光",
    description: "",
    aspect_ratio: "16:9",
    style_type: "realistic",
  });
  expect(onSubmit.mock.calls[0]?.[1]).toMatch(/^[a-f0-9-]{36}$/);
});
it("未确定结果保留内容和同次幂等键，修改输入换键且不自动重试", async () => {
  const onSubmit = vi
    .fn()
    .mockRejectedValue(new ApiError(0, "dependency_unavailable"));
  const { onCreated } = setup(onSubmit);
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "逆光" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await screen.findByText(/创建结果尚未确认/);
  expect(onSubmit).toHaveBeenCalledTimes(1);
  expect(onCreated).not.toHaveBeenCalled();
  expect((screen.getByLabelText("项目名称") as HTMLInputElement).value).toBe(
    "逆光",
  );
  const key = onSubmit.mock.calls[0][1];
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "逆光" },
  });
  fireEvent.click(screen.getByRole("button", { name: "重试创建" }));
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
  expect(onSubmit.mock.calls[1][1]).toBe(key);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "重试创建" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "逆光二" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(3));
  expect(onSubmit.mock.calls[2][1]).not.toBe(key);
});
it("待提交时禁止重复创建和放弃，失败后离开须确认", async () => {
  let reject!: (error: Error) => void;
  const onSubmit = vi.fn(
    () =>
      new Promise<{ id: string }>((_resolve, fail) => {
        reject = fail;
      }),
  );
  setup(onSubmit);
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "逆光" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  const confirmation = await screen.findByRole("dialog", {
    name: "离开项目创建？",
  });
  expect(
    within(confirmation)
      .getByRole("button", { name: "放弃并离开" })
      .hasAttribute("disabled"),
  ).toBe(true);
  reject(new ApiError(503, "dependency_unavailable"));
  await waitFor(() =>
    expect(
      within(confirmation)
        .getByRole("button", { name: "放弃并离开" })
        .hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.click(
    within(confirmation).getByRole("button", { name: "继续填写" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  fireEvent.click(screen.getByRole("button", { name: "放弃并离开" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

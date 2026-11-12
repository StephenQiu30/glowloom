import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { GenerationToolbarParameters } from "@/components/generation/generation-toolbar-parameters";

afterEach(cleanup);
it("数量独立选择，保留模型上限和其他参数且不提交表单", async () => {
  const change = vi.fn(),
    submit = vi.fn();
  render(
    <form onSubmit={submit}>
      <GenerationToolbarParameters
        enabled
        disabled={false}
        maxOutputs={2}
        params={{ output_count: 1, duration_ms: 5000 }}
        onChange={change}
      >
        参数内容
      </GenerationToolbarParameters>
    </form>,
  );
  fireEvent.click(screen.getByRole("button", { name: "生成数量" }));
  expect(await screen.findByRole("radio", { name: "2" })).toBeTruthy();
  expect(screen.queryByRole("radio", { name: "3" })).toBeNull();
  fireEvent.click(screen.getByRole("radio", { name: "2" }));
  expect(change).toHaveBeenCalledWith({ output_count: 2, duration_ms: 5000 });
  expect(submit).not.toHaveBeenCalled();
});
it("未选择模型不伪造数量时长，参数提示仍可查看", async () => {
  render(
    <GenerationToolbarParameters
      enabled={false}
      disabled={false}
      maxOutputs={8}
      params={{}}
      onChange={() => {}}
    />,
  );
  expect(screen.queryByRole("button", { name: "生成数量" })).toBeNull();
  expect(screen.queryByRole("button", { name: "视频时长" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "生成参数" }));
  expect(
    await screen.findByText("请先选择可用模型，再配置生成参数。"),
  ).toBeTruthy();
});

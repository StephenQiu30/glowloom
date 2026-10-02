import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BibleFields } from "./bible-fields";
import { definitionFields } from "./bible-model";
afterEach(cleanup);
it("完整角色11字段与含换行/空白别名无损保存，造型声音不从通用表单回写", async () => {
  const definition = Object.fromEntries(
    definitionFields.map(([key]) => [key, `  ${key}😀\n原描述  `]),
  );
  const initial = {
    name: "  正式角色😀\n保留  ",
    aliases: ["  别名1  ", "多行\n别名"],
    description: "原说明",
    definition,
  };
  const submit = vi.fn();
  render(
    <BibleFields
      kind="character"
      initial={initial}
      locked={false}
      onDirty={vi.fn()}
      onSubmit={submit}
    />,
  );
  for (const [, label] of definitionFields)
    expect(screen.getByRole("textbox", { name: label })).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "说明" }), {
    target: { value: "仅修改说明" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存完整设定" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      ...initial,
      description: "仅修改说明",
    }),
  );
});
it("场景与道具独立prompt，空名和重复别名阻断写入", async () => {
  const submit = vi.fn();
  render(
    <BibleFields
      kind="location"
      initial={{
        name: "场景😀",
        aliases: ["重复", "重复"],
        prompt: "原prompt\n😀",
      }}
      locked={false}
      onDirty={vi.fn()}
      onSubmit={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "保存完整设定" }));
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "移除别名2" }));
  fireEvent.click(screen.getByRole("button", { name: "保存完整设定" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      name: "场景😀",
      aliases: ["重复"],
      prompt: "原prompt\n😀",
    }),
  );
});

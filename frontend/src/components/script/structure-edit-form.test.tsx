import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StructureEditForm } from "./structure-edit-form";
vi.mock("./character-binding-picker", () => ({
  CharacterBindingPicker: ({
    label,
    binding,
    locked,
    onChange,
  }: {
    label: string;
    binding?: { character_id: string; character_version_id?: string };
    locked: boolean;
    onChange: (
      binding:
        { character_id: string; character_version_id?: string } | undefined,
    ) => void;
  }) => (
    <select
      aria-label={`${label}正式确认角色`}
      disabled={locked}
      value={binding?.character_id ?? ""}
      onChange={(event) =>
        onChange(
          event.target.value
            ? {
                character_id: event.target.value,
                character_version_id: "66666666-6666-4666-8666-666666666666",
              }
            : undefined,
        )
      }
    >
      <option value="">清除角色绑定</option>
      <option value="33333333-3333-4333-8333-333333333333">原角色</option>
      <option value="55555555-5555-4555-8555-555555555555">新确认角色</option>
    </select>
  ),
}));
afterEach(cleanup);
const scope = {
  origin: window.location.origin,
  actorId: "77777777-7777-4777-8777-777777777777",
  orgId: "88888888-8888-4888-8888-888888888888",
  projectId: "99999999-9999-4999-8999-999999999999",
};
const base = {
  expected_revision: 8,
  expected_episode_revision: 3,
  base_structure_version_no: 1,
};
const scene = {
  scene_key: "11111111-1111-4111-8111-111111111111",
  seq_no: 1,
  heading: "初场",
  location_text: "客厅",
  time_of_day: "日",
  span_start: 0,
  span_end: 9,
  items: [],
};
const line = {
  line_key: "22222222-2222-4222-8222-222222222222",
  content: "原话",
  span_start: 1,
  span_end: 3,
};
it.each(["unchanged", "changed", "cleared"])(
  "角色版本pin在%s身份编辑中保持正确",
  async (change) => {
    const submit = vi.fn();
    const character = "33333333-3333-4333-8333-333333333333";
    const pin = "44444444-4444-4444-8444-444444444444";
    const next = "55555555-5555-4555-8555-555555555555";
    render(
      <StructureEditForm
        scope={scope}
        base={base}
        episodeStart={0}
        episodeEnd={9}
        initialDocument={{
          scenes: [
            {
              ...scene,
              items: [
                {
                  ...line,
                  type: "line",
                  kind: "dialogue",
                  character_id: character,
                  character_version_id: pin,
                },
              ],
            },
          ],
          unassigned_lines: [],
        }}
        locked={false}
        onDirty={vi.fn()}
        onSubmit={submit}
      />,
    );
    fireEvent.change(screen.getByRole("textbox", { name: "场景1标题" }), {
      target: { value: "新场名" },
    });
    if (change !== "unchanged")
      fireEvent.change(
        screen.getByRole("combobox", { name: "场景1行1正式确认角色" }),
        { target: { value: change === "changed" ? next : "" } },
      );
    fireEvent.click(screen.getByRole("button", { name: "保存手工结构候选" }));
    await waitFor(() => expect(submit).toHaveBeenCalledOnce());
    const saved = submit.mock.calls[0][0].document.scenes[0].items[0];
    expect(saved.character_id).toBe(
      change === "unchanged"
        ? character
        : change === "changed"
          ? next
          : undefined,
    );
    expect(saved.character_version_id).toBe(
      change === "unchanged"
        ? pin
        : change === "changed"
          ? "66666666-6666-4666-8666-666666666666"
          : undefined,
    );
    expect(saved.line_key).toBe(line.line_key);
    expect(saved.content).toBe(line.content);
  },
);
it("未归属行显式分配仍保持稳定身份与原坐标，保存完整结构和原CAS", async () => {
  const submit = vi.fn();
  render(
    <StructureEditForm
      scope={scope}
      base={base}
      episodeStart={0}
      episodeEnd={9}
      initialDocument={{ scenes: [scene], unassigned_lines: [line] }}
      locked={false}
      onDirty={vi.fn()}
      onSubmit={submit}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "将未归属行1分配到所选场景" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存手工结构候选" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      ...base,
      document: {
        scenes: [
          { ...scene, items: [{ ...line, type: "line", kind: "dialogue" }] },
        ],
        unassigned_lines: [],
      },
    }),
  );
});
it("场景区间错误不发送且保留输入；unknown禁用全部写入", async () => {
  const submit = vi.fn();
  const props = {
    scope,
    base,
    episodeStart: 0,
    episodeEnd: 9,
    initialDocument: { scenes: [scene], unassigned_lines: [line] },
    locked: false,
    onDirty: vi.fn(),
    onSubmit: submit,
  };
  const ui = render(<StructureEditForm {...props} />);
  const end = screen.getByRole("spinbutton", { name: "场景1正文终点" });
  fireEvent.change(end, { target: { value: "10" } });
  fireEvent.click(screen.getByRole("button", { name: "保存手工结构候选" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(submit).not.toHaveBeenCalled();
  expect((end as HTMLInputElement).value).toBe("10");
  ui.rerender(<StructureEditForm {...props} locked />);
  expect(end.matches(":disabled")).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "保存手工结构候选" })
      .hasAttribute("disabled"),
  ).toBe(true);
});

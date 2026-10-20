import { render, findFeedback } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SubjectEditor } from "@/components/subjects/subject-editor";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  get: vi.fn(),
  refs: vi.fn(),
}));
vi.mock("@/api/bible", () => ({
  createBibleEntry: api.create,
  updateBibleEntry: api.update,
  getBibleEntry: api.get,
  replaceBibleReferences: api.refs,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => <span>参考图</span>,
  ProjectMediaPicker: ({
    onSelect,
  }: {
    onSelect: (a: { id: string }) => void;
  }) => (
    <button
      onClick={() => onSelect({ id: "44444444-4444-4444-8444-444444444444" })}
    >
      测试素材
    </button>
  ),
}));
const project = "11111111-1111-4111-8111-111111111111",
  id = "22222222-2222-4222-8222-222222222222",
  version = "33333333-3333-4333-8333-333333333333",
  look = "55555555-5555-4555-8555-555555555555";
const detail = {
  head: {
    id,
    project_id: project,
    kind: "character",
    revision: 1,
    current_version_id: version,
  },
  current: {
    id: version,
    entry_id: id,
    project_id: project,
    character: {
      name: "角色",
      description: "保留描述",
      looks: [{ id: look, default: true, references: [] }],
    },
  },
};
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);
it("参考图保存断网后继续原主体，不再创建新身份或换请求号", async () => {
  api.create.mockResolvedValue({
    entry_id: id,
    kind: "character",
    revision: 1,
    version_id: version,
  });
  api.get
    .mockResolvedValueOnce(detail)
    .mockResolvedValueOnce(detail)
    .mockResolvedValue({
      ...detail,
      head: { ...detail.head, revision: 2 },
      current: {
        ...detail.current,
        character: {
          ...detail.current.character,
          description: undefined,
          looks: [
            {
              id: look,
              default: true,
              references: [
                {
                  role: "primary",
                  media: { asset_id: "44444444-4444-4444-8444-444444444444" },
                },
              ],
            },
          ],
        },
      },
    });
  api.refs
    .mockRejectedValueOnce(new RequestError("request_unconfirmed"))
    .mockResolvedValue({
      entry_id: id,
      kind: "character",
      revision: 2,
      version_id: version,
    });
  const saved = vi.fn();
  render(
    <SubjectEditor
      projectID={project}
      kind="character"
      onClose={() => {}}
      onSaved={saved}
    />,
  );
  fireEvent.change(screen.getByLabelText("主体名称"), {
    target: { value: "角色" },
  });
  fireEvent.click(screen.getByRole("button", { name: "选择主体参考图" }));
  fireEvent.click(screen.getByText("测试素材"));
  fireEvent.click(screen.getByRole("button", { name: "保存主体" }));
  await findFeedback();
  fireEvent.click(screen.getByRole("button", { name: "重试原保存" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.create).toHaveBeenCalledOnce();
  expect(api.refs).toHaveBeenCalledTimes(2);
  expect(api.refs.mock.calls[0][2]).toEqual(api.refs.mock.calls[1][2]);
});

it.each(["location", "prop"] as const)(
  "%s 创建时兼容服务端省略空描述",
  async (kind) => {
    const result = {
      ...detail,
      head: { ...detail.head, kind },
      current: { ...detail.current, [kind]: { name: "主体" } },
    };
    api.create.mockResolvedValue({
      entry_id: id,
      kind,
      revision: 1,
      version_id: version,
    });
    api.get.mockResolvedValue(result);
    const saved = vi.fn();
    render(
      <SubjectEditor
        projectID={project}
        kind={kind}
        onClose={() => {}}
        onSaved={saved}
      />,
    );
    fireEvent.change(screen.getByLabelText("主体名称"), {
      target: { value: "主体" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存主体" }));
    await waitFor(() => expect(saved).toHaveBeenCalledOnce());
    expect(api.create).toHaveBeenCalledOnce();
  },
);
it("清空已有描述后按真实 omitempty 回执完成保存", async () => {
  const saved = vi.fn();
  const initial = {
    ...detail,
    head: { ...detail.head, kind: "prop" as const },
    current: {
      ...detail.current,
      prop: { name: "物品", description: "原描述" },
    },
  };
  api.update.mockResolvedValue({
    entry_id: id,
    kind: "prop",
    revision: 2,
    version_id: version,
  });
  api.get.mockResolvedValue({
    ...initial,
    head: { ...initial.head, revision: 2 },
    current: { ...initial.current, prop: { name: "物品" } },
  });
  render(
    <SubjectEditor
      projectID={project}
      kind="prop"
      initial={initial}
      onClose={() => {}}
      onSaved={saved}
    />,
  );
  fireEvent.change(screen.getByLabelText("主体描述"), {
    target: { value: "" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存主体" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
});

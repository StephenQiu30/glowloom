import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { ScriptPanel } from "@/app/short-drama/script-panel";
import { RequestError } from "@/lib/request";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  upload: vi.fn(),
  parse: vi.fn(),
  list: vi.fn(),
}));
vi.mock("@/api/script", () => ({
  createScriptSource: api.create,
  createScriptFileImport: api.parse,
  listScriptFileImports: api.list,
}));
vi.mock("@/api/media", () => ({ uploadMediaAsset: api.upload }));
const project = "11111111-1111-4111-8111-111111111111";
const resource = "22222222-2222-4222-8222-222222222222";

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function setup() {
  api.list.mockResolvedValue({ items: [] });
  const refresh = vi.fn(async () => {});
  render(
    <SWRConfig value={{ provider: () => new Map(), shouldRetryOnError: false }}>
      <ScriptPanel
        projectID={project}
        owner="test-owner"
        head={{ project_id: project, revision: 0 }}
        writable
        onChange={refresh}
        onLock={() => {}}
      />
    </SWRConfig>,
  );
  return refresh;
}

it("粘贴剧本受理结果未知时重放原内容与幂等键", async () => {
  const refresh = setup();
  api.create
    .mockRejectedValueOnce(new RequestError("timeout", 503))
    .mockResolvedValueOnce({ version_id: resource, script_revision: 1 });
  const text = screen.getByLabelText("粘贴文本");
  fireEvent.change(text, { target: { value: "第1集\n镜头一：窗外有雪。" } });
  const save = screen.getByRole("button", { name: "保存为剧本草稿" });
  expect(save.hasAttribute("disabled")).toBe(true);
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(save);
  fireEvent.click(await screen.findByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
  expect(api.create).toHaveBeenCalledTimes(2);
  expect(api.create.mock.calls[0].slice(0, 2)).toEqual(
    api.create.mock.calls[1].slice(0, 2),
  );
  expect(api.create.mock.calls[0][2].headers).toEqual(
    api.create.mock.calls[1][2].headers,
  );
  expect(api.create.mock.calls[0][1]).toMatchObject({
    expected_revision: 0,
    rights_confirmed: true,
    document: {
      content: [
        { type: "paragraph", content: [{ type: "text", text: "第1集" }] },
        {
          type: "paragraph",
          content: [{ type: "text", text: "镜头一：窗外有雪。" }],
        },
      ],
    },
  });
});

it("已上传原件在解析受理未知时只重放解析请求", async () => {
  setup();
  api.upload.mockResolvedValue({
    asset: { id: resource, project_id: project, kind: "document" },
  });
  api.parse
    .mockRejectedValueOnce(new RequestError("timeout", 503))
    .mockResolvedValueOnce({
      id: resource,
      project_id: project,
      status: "queued",
    });
  fireEvent.change(screen.getByLabelText("上传文件"), {
    target: {
      files: [new File(["第一集"], "script.txt", { type: "text/plain" })],
    },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传并解析" }));
  fireEvent.click(await screen.findByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(api.parse).toHaveBeenCalledTimes(2));
  expect(api.upload).toHaveBeenCalledOnce();
  expect(api.parse.mock.calls[1][1]).toMatchObject({ asset_ids: [resource] });
  expect(api.parse.mock.calls[0][2].headers).toEqual(
    api.parse.mock.calls[1][2].headers,
  );
});

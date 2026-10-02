import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ScriptHistoryDialog } from "./script-history-dialog";
import * as review from "./review-queries";
vi.mock("./rich-source-editor", () => ({
  RichSourceEditor: ({ disabled }: { disabled: boolean }) => (
    <p>{disabled ? "只读格式原文" : "不应编辑"}</p>
  ),
}));
vi.mock("./review-queries", async (original) => ({
  ...(await original<typeof import("./review-queries")>()),
  listVersions: vi.fn(),
  listSourceHistory: vi.fn(),
  getSourceSnapshot: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const source = {
  id: id(4),
  source_lineage_id: id(5),
  source_revision: 2,
  source_kind: "chapter" as const,
  title: "被替代章节",
  status: "ready" as const,
  origin: "manual" as const,
  position: 0,
  char_count: 0,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
  document: { type: "doc" as const },
  plain_text: "",
  provenance: {
    warnings: [{ code: "table_layout_flattened" as const, count: 3 }],
  },
  original_html: "<script>neverExecute()</script><p>原HTML</p>",
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(review.listVersions).mockResolvedValue({ items: [] });
  vi.mocked(review.listSourceHistory).mockResolvedValue({ items: [source] });
  vi.mocked(review.getSourceSnapshot).mockResolvedValue(source);
});
afterEach(cleanup);
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ScriptHistoryDialog
        scope={scope}
        lineageId={id(5)}
        onClose={vi.fn()}
        onChooseVersion={vi.fn()}
      />
    </QueryClientProvider>,
  );
}
it("完整历史只预读摘要，选中被移除快照才懒读正文；原HTML逐字只读而不执行", async () => {
  const ui = mount();
  const select = await screen.findByRole("button", {
    name: "被替代章节 · 来源版本 2 · 0 字符",
  });
  expect(review.getSourceSnapshot).not.toHaveBeenCalled();
  fireEvent.click(select);
  expect(await screen.findByText("只读格式原文")).toBeTruthy();
  expect(screen.getByText("表格布局已展开为正文 · 3 处")).toBeTruthy();
  expect(review.getSourceSnapshot).toHaveBeenCalledWith(
    scope,
    source.id,
    expect.any(AbortSignal),
  );
  fireEvent.click(screen.getByText("逐字查看原始 HTML（只读）"));
  await waitFor(() =>
    expect(screen.getByText(source.original_html)).toBeTruthy(),
  );
  expect(ui.container.querySelector("script")).toBeNull();
});
it("跨页重复快照身份明确拒绝展示，不把摘要错误当缺正文成功", async () => {
  vi.mocked(review.listSourceHistory).mockResolvedValue({
    items: [source, source],
  });
  mount();
  expect(
    await screen.findByText("来源历史分页出现重复身份，请重新读取。"),
  ).toBeTruthy();
  expect(review.getSourceSnapshot).not.toHaveBeenCalled();
  expect(
    screen.queryByRole("button", { name: "被替代章节 · 来源版本 2 · 0 字符" }),
  ).toBeNull();
});

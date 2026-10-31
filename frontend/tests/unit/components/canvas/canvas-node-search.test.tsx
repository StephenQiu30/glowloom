import { Activity } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasNodeSearch } from "@/components/canvas/search/canvas-node-search";
import { searchCanvasNodes } from "@/components/canvas/search/canvas-node-search-state";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
const previewAPI = vi.hoisted(() => ({
  session: "session",
  media: vi.fn(),
  version: vi.fn(),
  binding: vi.fn(),
}));
vi.mock("@/api/media", () => ({ previewLibraryMediaAsset: previewAPI.media }));
vi.mock("@/api/subjects", () => ({
  getSubjectVersion: previewAPI.version,
  getSubjectMedia: previewAPI.binding,
}));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("project_id=project"),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({
    session_id: previewAPI.session,
    actor: { id: "actor" },
  }),
}));
vi.mock("@/components/canvas/lib/canvas-subject-source", () => ({
  requireCanvasSubjectVersion: (value: unknown) => value,
}));
vi.mock("@/lib/request", () => ({
  getCreationScope: () => "scope",
  getScopeGeneration: () => 1,
  getSessionGeneration: () => 1,
  subscribeContextChange: () => () => {},
}));
afterEach(() => {
  cleanup();
  previewAPI.session = "session";
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});
const make = (type: string, title = type): CanvasNodeData => ({
  id: `node-${type}`,
  type,
  title,
  position: { x: 0, y: 0 },
  width: 100,
  height: 100,
});
const nodes = [
  "image",
  "video",
  "audio",
  "text",
  "subject",
  "timeline",
  "director",
  "group",
  "custom:node",
].map((type) => make(type));
it("keeps source cinema_studio distinct from external director and counts every node once", () => {
  const state = searchCanvasNodes(nodes, "", "all");
  expect(state.counts).toEqual({
    all: 9,
    image: 1,
    video: 1,
    audio: 1,
    text: 1,
    subject: 1,
    timeline: 1,
    cinema_studio: 0,
    group: 1,
    stack: 0,
    other: 2,
  });
  expect(
    searchCanvasNodes([make("image_generation")], "", "image").counts.image,
  ).toBe(1);
  expect(
    searchCanvasNodes([make("cinema_studio")], "", "cinema_studio").counts
      .cinema_studio,
  ).toBe(1);
  expect(
    searchCanvasNodes([make("director")], "", "cinema_studio").results,
  ).toEqual([]);
  expect(searchCanvasNodes(nodes, "", "other").results.at(-1)?.type).toBe(
    "custom:node",
  );
});
it("matches current names only, preserving same-name nodes and excluding body, prompt, tags and UUID", () => {
  const a = {
    ...make("text", "  海边 SUN  "),
    metadata: {
      content: "秘密正文",
      prompt: "隐含提示",
      colorTags: ["cyan" as const],
    },
  };
  const b = { ...make("image", "海边 sun"), id: "different-id" };
  expect(searchCanvasNodes([a, b], " SUN ", "all").results).toHaveLength(2);
  for (const query of ["秘密正文", "隐含提示", "cyan", a.id])
    expect(searchCanvasNodes([a, b], query, "all").results).toHaveLength(0);
  expect(searchCanvasNodes([a, b], "sun", "text").counts).toMatchObject({
    all: 2,
    text: 1,
    image: 1,
  });
});
it("shows live counts, names and type, filters and clears, then locates the exact result", async () => {
  const locate = vi.fn();
  render(
    <CanvasNodeSearch nodes={nodes} onLocate={locate}>
      <button>打开搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "打开搜索" }));
  const dialog = screen.getByRole("dialog");
  expect(dialog.getAttribute("data-slot")).toBe("popover-content");
  expect(
    screen.getByRole("tab", { name: "导演工具箱，0 个节点" }),
  ).toBeTruthy();
  expect(screen.getByRole("tab", { name: "其他，2 个节点" })).toBeTruthy();
  expect(
    within(dialog).getByRole("tab", { name: "主体，1 个节点" }),
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText("节点名称"), {
    target: { value: "subject" },
  });
  expect(screen.getByRole("status").textContent).toBe("1 个结果，共 9 个节点");
  expect(screen.getByRole("tab", { name: "全部，1 个节点" })).toBeTruthy();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "视频，0 个节点" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(screen.getByText("没有找到节点，请调整名称或类型筛选。")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "清空节点搜索" }));
  expect(
    screen
      .getByRole("tab", { name: "视频，1 个节点" })
      .getAttribute("aria-selected"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "定位视频节点：video" }));
  await waitFor(() =>
    expect(locate).toHaveBeenCalledExactlyOnceWith("node-video"),
  );
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("updates renamed/deleted results and returns focus on Escape without locating", async () => {
  const locate = vi.fn();
  const view = render(
    <CanvasNodeSearch nodes={[make("text", "旧名")]} onLocate={locate}>
      <button>搜索入口</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索入口" }));
  view.rerender(
    <CanvasNodeSearch nodes={[make("text", "新名")]} onLocate={locate}>
      <button>搜索入口</button>
    </CanvasNodeSearch>,
  );
  expect(
    screen.queryByRole("button", { name: "定位文本节点：旧名" }),
  ).toBeNull();
  expect(
    screen.getByRole("button", { name: "定位文本节点：新名" }),
  ).toBeTruthy();
  view.rerender(
    <CanvasNodeSearch nodes={[]} onLocate={locate}>
      <button>搜索入口</button>
    </CanvasNodeSearch>,
  );
  expect(screen.getByText("当前画布暂无节点。")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "搜索入口" }),
    ),
  );
  expect(locate).not.toHaveBeenCalled();
});

it("classifies typed stacks separately from legacy collapsed groups", () => {
  const ordinary = {
    ...nodes.find((n) => n.type === "group")!,
    id: crypto.randomUUID(),
    documentData: { collapsed: true } as never,
  };
  const stack = {
    ...ordinary,
    id: crypto.randomUUID(),
    serviceData: {
      config: {
        container: {
          version: 1,
          kind: "stack" as const,
          background: "none" as const,
        },
      },
    },
  };
  const result = searchCanvasNodes([ordinary, stack], "", "stack");
  expect(result.counts.group).toBe(1);
  expect(result.counts.stack).toBe(1);
  expect(result.results).toEqual([stack]);
});

const videoResult = (id = "video") => ({
  ...make("video", id),
  id,
  documentData: {
    fileReferences: [{ purpose: "CONTENT", fileId: "asset", version: "2" }],
  } as never,
});
it("resolves authorized poster once for duplicate visible assets and falls back on image failure", async () => {
  previewAPI.media.mockResolvedValue({
    asset: { id: "asset", project_id: "project", revision: 2, kind: "video" },
    url: "https://media.test/original.mp4",
    expires_at: new Date(Date.now() + 60000).toISOString(),
    renditions: [
      {
        kind: "poster",
        url: "https://media.test/poster.jpg",
        expires_at: new Date(Date.now() + 60000).toISOString(),
      },
    ],
  });
  const view = render(
    <CanvasNodeSearch
      nodes={[videoResult("a"), videoResult("b")]}
      onLocate={vi.fn()}
    >
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  expect(previewAPI.media).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() =>
    expect(
      view.container.ownerDocument.querySelectorAll(
        'img[src="https://media.test/poster.jpg"]',
      ),
    ).toHaveLength(2),
  );
  expect(previewAPI.media).toHaveBeenCalledTimes(1);
  expect(previewAPI.media.mock.calls[0][0]).toEqual({
    scope: "project",
    project_id: "project",
    item_id: "asset",
  });
  expect(document.querySelector("video")).toBeNull();
  fireEvent.error(document.querySelector("img")!);
  expect(document.querySelectorAll("img")).toHaveLength(1);
});
it("does not show unauthorized, expired, changed or missing poster grants", async () => {
  for (const response of [
    null,
    {
      asset: { id: "asset", project_id: "project", revision: 3, kind: "video" },
    },
    {
      asset: { id: "asset", project_id: "project", revision: 2, kind: "video" },
      url: "https://media.test/original.mp4",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    },
    {
      asset: { id: "asset", project_id: "project", revision: 2, kind: "video" },
      renditions: [
        {
          kind: "poster",
          url: "https://media.test/expired.jpg",
          expires_at: new Date(0).toISOString(),
        },
      ],
    },
  ]) {
    if (response) previewAPI.media.mockResolvedValueOnce(response);
    else previewAPI.media.mockRejectedValueOnce(new Error("forbidden"));
    render(
      <CanvasNodeSearch nodes={[videoResult()]} onLocate={vi.fn()}>
        <button>搜索</button>
      </CanvasNodeSearch>,
    );
    fireEvent.click(screen.getByRole("button", { name: "搜索" }));
    await waitFor(() => expect(previewAPI.media).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.querySelector("img")).toBeNull();
    expect(
      screen.getByRole("button", { name: "定位视频节点：video" }),
    ).toBeTruthy();
    cleanup();
    vi.clearAllMocks();
  }
});

it("only requests intersecting rows, limits concurrent requests and cancels on close", async () => {
  const observers: {
    callback: IntersectionObserverCallback;
    element?: Element;
  }[] = [];
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      entry: (typeof observers)[number];
      constructor(callback: IntersectionObserverCallback) {
        this.entry = { callback };
        observers.push(this.entry);
      }
      observe(element: Element) {
        this.entry.element = element;
      }
      disconnect() {}
    },
  );
  previewAPI.media.mockImplementation(() => new Promise(() => {}));
  const list = Array.from({ length: 20 }, (_, i) => ({
    ...videoResult(`v${i}`),
    documentData: {
      fileReferences: [
        { purpose: "CONTENT", fileId: `asset${i}`, version: "2" },
      ],
    } as never,
  }));
  render(
    <CanvasNodeSearch nodes={list} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  expect(previewAPI.media).not.toHaveBeenCalled();
  act(() =>
    observers.slice(0, 6).forEach((item) =>
      item.callback(
        [
          {
            isIntersecting: true,
            target: item.element,
          } as IntersectionObserverEntry,
        ],
        {} as IntersectionObserver,
      ),
    ),
  );
  expect(previewAPI.media).toHaveBeenCalledTimes(3);
  const signals = previewAPI.media.mock.calls.map(
    (call) => call[1].signal as AbortSignal,
  );
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() =>
    expect(signals.every((signal) => signal.aborted)).toBe(true),
  );
});
it("deduplicates fixed subject primaries and revokes owned blobs when search closes", async () => {
  const create = vi.fn(() => "blob:subject-preview"),
    revoke = vi.fn();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = create;
      static revokeObjectURL = revoke;
    },
  );
  previewAPI.version.mockResolvedValue({
    primary: {
      binding_id: "binding",
      asset_id: "asset",
      asset_revision: 2,
      kind: "image",
    },
    references: [],
  });
  previewAPI.binding.mockResolvedValue(
    new Blob(["image"], { type: "image/png" }),
  );
  const subject = {
    ...make("subject"),
    serviceData: {
      config: {
        subject: { version: 1, subject_id: "subject", version_id: "version" },
      },
    },
  };
  render(
    <CanvasNodeSearch
      nodes={[subject, { ...subject, id: "subject-copy" }]}
      onLocate={vi.fn()}
    >
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() =>
    expect(
      document.querySelectorAll('img[src="blob:subject-preview"]'),
    ).toHaveLength(2),
  );
  expect(previewAPI.version).toHaveBeenCalledTimes(1);
  expect(previewAPI.binding).toHaveBeenCalledExactlyOnceWith(
    { id: "subject", version: "version", binding: "binding" },
    expect.objectContaining({ responseType: "blob" }),
  );
  expect(previewAPI.media).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() =>
    expect(revoke).toHaveBeenCalledExactlyOnceWith("blob:subject-preview"),
  );
});
it("prioritizes local primary and leaves cleared or bound-video subjects as type icons", async () => {
  previewAPI.media.mockResolvedValue({
    asset: { id: "local", revision: 3, kind: "image" },
    url: "https://media.test/local.jpg",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  });
  previewAPI.version.mockResolvedValue({
    primary: {
      binding_id: "binding",
      asset_id: "video",
      asset_revision: 1,
      kind: "video",
    },
    references: [],
  });
  const pin = { version: 1, subject_id: "subject", version_id: "version" };
  const list = [
    {
      ...make("subject", "局部"),
      id: "local",
      serviceData: {
        config: {
          subject: {
            ...pin,
            local: {
              description: "",
              references: [],
              primary: { asset_id: "local", asset_revision: 3, kind: "image" },
            },
          },
        },
      },
    },
    {
      ...make("subject", "已清除"),
      id: "clear",
      serviceData: {
        config: {
          subject: { ...pin, local: { description: "", references: [] } },
        },
      },
    },
    {
      ...make("subject", "视频主体"),
      id: "video",
      serviceData: { config: { subject: pin } },
    },
  ];
  render(
    <CanvasNodeSearch nodes={list} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() => expect(document.querySelectorAll("img")).toHaveLength(1));
  expect(document.querySelector("img")?.src).toBe(
    "https://media.test/local.jpg",
  );
  expect(previewAPI.version).toHaveBeenCalledTimes(1);
  expect(previewAPI.binding).not.toHaveBeenCalled();
});

it("removes an expired displayed grant and never substitutes its video original", async () => {
  previewAPI.media.mockResolvedValue({
    asset: { id: "asset", revision: 2, kind: "video" },
    url: "https://media.test/original.mp4",
    renditions: [
      {
        kind: "poster",
        url: "https://media.test/short.jpg",
        expires_at: new Date(Date.now() + 150).toISOString(),
      },
    ],
  });
  render(
    <CanvasNodeSearch nodes={[videoResult()]} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() =>
    expect(document.querySelector("img")?.src).toBe(
      "https://media.test/short.jpg",
    ),
  );
  fireEvent.load(document.querySelector("img")!);
  await waitFor(() => expect(document.querySelector("img")).toBeNull());
  expect(document.querySelector("video")).toBeNull();
});
it("ignores a late subject blob after its search owner closes", async () => {
  const create = vi.fn(),
    revoke = vi.fn();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = create;
      static revokeObjectURL = revoke;
    },
  );
  previewAPI.version.mockResolvedValue({
    primary: {
      binding_id: "binding",
      asset_id: "asset",
      asset_revision: 2,
      kind: "image",
    },
    references: [],
  });
  let finish!: (value: Blob) => void;
  previewAPI.binding.mockImplementation(
    () =>
      new Promise<Blob>((resolve) => {
        finish = resolve;
      }),
  );
  const subject = {
    ...make("subject"),
    serviceData: {
      config: {
        subject: { version: 1, subject_id: "subject", version_id: "version" },
      },
    },
  };
  render(
    <CanvasNodeSearch nodes={[subject]} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() => expect(previewAPI.binding).toHaveBeenCalled());
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await act(async () => {
    await Promise.resolve();
    finish(new Blob(["late"], { type: "image/png" }));
  });
  expect(create).not.toHaveBeenCalled();
  expect(document.querySelector("img")).toBeNull();
});

it("hides prior private previews immediately when the session changes", async () => {
  previewAPI.media
    .mockResolvedValueOnce({
      asset: { id: "asset", revision: 2, kind: "video" },
      renditions: [
        {
          kind: "poster",
          url: "https://media.test/old-session.jpg",
          expires_at: new Date(Date.now() + 60000).toISOString(),
        },
      ],
    })
    .mockImplementationOnce(() => new Promise(() => {}));
  const view = render(
    <CanvasNodeSearch nodes={[videoResult()]} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() => expect(document.querySelector("img")).not.toBeNull());
  previewAPI.session = "new-session";
  view.rerender(
    <CanvasNodeSearch nodes={[videoResult()]} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>,
  );
  expect(document.querySelector("img")).toBeNull();
  await waitFor(() => expect(previewAPI.media).toHaveBeenCalledTimes(2));
});

it("reauthorizes previews after Activity hides and reveals the search", async () => {
  previewAPI.media.mockResolvedValue({
    asset: { id: "asset", revision: 2, kind: "video" },
    renditions: [
      {
        kind: "poster",
        url: "https://media.test/revealed.jpg",
        expires_at: new Date(Date.now() + 60000).toISOString(),
      },
    ],
  });
  const content = (
    <CanvasNodeSearch nodes={[videoResult()]} onLocate={vi.fn()}>
      <button>搜索</button>
    </CanvasNodeSearch>
  );
  const view = render(<Activity mode="visible">{content}</Activity>);
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() => expect(document.querySelector("img")).not.toBeNull());
  view.rerender(<Activity mode="hidden">{content}</Activity>);
  await act(async () => {
    await Promise.resolve();
  });
  expect(previewAPI.media.mock.calls[0][1].signal.aborted).toBe(true);
  view.rerender(<Activity mode="visible">{content}</Activity>);
  await waitFor(() => expect(previewAPI.media).toHaveBeenCalledTimes(2));
  expect(previewAPI.media.mock.calls[1][1].signal.aborted).toBe(false);
});

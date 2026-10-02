import { describe, expect, it } from "vitest";
import {
  libraryCommandSchema,
  readLibraryPage,
  readLibraryReceipt,
  readLibraryDetail,
  folderAncestry,
  canReparentLibraryFolder,
} from "./library-model";

const actor = "11111111-1111-4111-8111-111111111111";
const org = "22222222-2222-4222-8222-222222222222";
const library = "33333333-3333-5333-8333-333333333333";
const id = "44444444-4444-4444-8444-444444444444";
const parent = "55555555-5555-4555-8555-555555555555";
const time = "2026-10-02T00:00:00Z";
const item = {
  id,
  asset_id: null,
  folder_id: null,
  kind: "text",
  title: "正文",
  category: "material",
  tags: [],
  source_label: "手工",
  note: "",
  favorite: false,
  catalog_state: "active",
  trashed_at: null,
  position: 0,
  revision: 1,
  created_at: time,
  updated_at: time,
};
const page = {
  current_actor_id: actor,
  current_org_id: org,
  library_id: library,
  scope: { kind: "personal" },
  revision: 1,
  page: 1,
  page_size: 40,
  total: 1,
  items: [item],
  category_counts: { material: 1 },
  folder_counts: { root: 1 },
  folders: [],
};
const folder = {
  id: parent,
  library_id: library,
  library_kind: "personal",
  parent_id: null,
  name: "😀".repeat(40),
  position: 0,
  style: "",
  theme: "",
  revision: 1,
  created_at: time,
  updated_at: time,
};
const scope = { kind: "personal" as const };
const identity = {
  origin: "http://127.0.0.1:3000",
  actorId: actor,
  orgId: org,
  libraryId: library,
  scope,
};
it("移动整棵目录不能制造9层或环，根目录保留子树可行", () => {
  const tree = Array.from({ length: 8 }, (_, index) => ({
    ...folder,
    id: `0000000${index + 1}-aaaa-4aaa-8aaa-aaaaaaaaaaaa`,
    parent_id: index ? `0000000${index}-aaaa-4aaa-8aaa-aaaaaaaaaaaa` : null,
    library_kind: "project" as const,
    name: `层${index + 1}`,
    style: "paper",
    theme: "pearl",
  }));
  const other = { ...tree[0], id: actor, parent_id: null };
  const all = [...tree, other];
  expect(canReparentLibraryFolder(all, tree[0].id, other.id)).toBe(false);
  expect(canReparentLibraryFolder(all, tree[0].id, tree[1].id)).toBe(false);
  expect(canReparentLibraryFolder(all, tree[1].id, other.id)).toBe(true);
  expect(canReparentLibraryFolder(all, tree[0].id, null)).toBe(true);
  expect(canReparentLibraryFolder(all, undefined, tree[7].id)).toBe(false);
});
describe("正式素材库读取边界", () => {
  it("页码、查询和当前主体严格匹配，非法事实不会进入缓存", () => {
    expect(
      readLibraryPage(page, scope, { page: 1, page_size: 40 }).items[0].title,
    ).toBe("正文");
    for (const invalid of [
      { ...page, page: 2 },
      { ...page, scope: { kind: "project", project_id: id } },
      { ...page, items: [item, item] },
      { ...page, items: [{ ...item, object_key: "private" }] },
      { ...page, category_counts: { provider: 1 } },
      { ...page, folder_counts: { [parent]: 1 } },
    ])
      expect(() =>
        readLibraryPage(invalid, scope, { page: 1, page_size: 40 }),
      ).toThrow();
    expect(() =>
      readLibraryPage(
        page,
        scope,
        { page: 1, page_size: 40 },
        { ...identity, actorId: id },
      ),
    ).toThrow();
  });
  it("个人40个Unicode字符，项目8层完整树，跨库/环/缺父级拒绝", () => {
    expect(
      readLibraryPage({ ...page, folders: [folder] }, scope, {
        page: 1,
        page_size: 40,
      }).folders[0].name,
    ).toBe(folder.name);
    expect(() =>
      readLibraryPage(
        { ...page, folders: [{ ...folder, name: folder.name + "😀" }] },
        scope,
        { page: 1, page_size: 40 },
      ),
    ).toThrow();
    const projectScope = { kind: "project" as const, project_id: id };
    const folders = Array.from({ length: 8 }, (_, i) => ({
      ...folder,
      id: `0000000${i + 1}-aaaa-4aaa-8aaa-aaaaaaaaaaaa`,
      parent_id: i ? `0000000${i}-aaaa-4aaa-8aaa-aaaaaaaaaaaa` : null,
      library_kind: "project",
      name: `层${i + 1}`,
      style: "paper",
      theme: "pearl",
    }));
    const tree = readLibraryPage(
      { ...page, scope: projectScope, folders },
      projectScope,
      { page: 1, page_size: 40 },
    );
    expect(folderAncestry(tree.folders, folders[7].id)).toHaveLength(8);
    expect(() =>
      readLibraryPage(
        {
          ...page,
          scope: projectScope,
          folders: [
            ...folders,
            { ...folders[7], id: parent, parent_id: folders[7].id },
          ],
        },
        projectScope,
        { page: 1, page_size: 40 },
      ),
    ).toThrow();
    expect(() =>
      readLibraryPage(
        {
          ...page,
          scope: projectScope,
          folders: [{ ...folders[0], parent_id: parent }],
        },
        projectScope,
        { page: 1, page_size: 40 },
      ),
    ).toThrow();
    expect(() =>
      readLibraryPage(
        { ...page, folders: [{ ...folder, library_id: id }] },
        scope,
        { page: 1, page_size: 40 },
      ),
    ).toThrow();
  });
  it("正文与二进制互斥，零metadata版本仅真实legacy asset有效", () => {
    expect(
      readLibraryDetail({ ...item, plain_text: "😀\r\n é" }, id).plain_text,
    ).toBe("😀\r\n é");
    expect(() =>
      readLibraryDetail({ ...item, asset_id: id, plain_text: "伪正文" }, id),
    ).toThrow();
    expect(() =>
      readLibraryDetail({ ...item, revision: 0, plain_text: "x" }, id),
    ).toThrow();
  });
});
describe("闭合命令与永久回执", () => {
  const body = {
    scope,
    action: "move_items" as const,
    expected_revision: 1,
    items: [{ id, revision: 1 }],
    target_folder_id: null,
  };
  it("批量≤200唯一ID并保存根目录显式null；个人不允许项目移除", () => {
    expect(libraryCommandSchema.parse(body)).toMatchObject({
      target_folder_id: null,
    });
    expect(() =>
      libraryCommandSchema.parse({
        ...body,
        items: [
          { id, revision: 1 },
          { id, revision: 2 },
        ],
      }),
    ).toThrow();
    expect(() =>
      libraryCommandSchema.parse({
        ...body,
        action: "remove_items",
        target_folder_id: undefined,
      }),
    ).toThrow();
    expect(() =>
      libraryCommandSchema.parse({
        ...body,
        items: Array.from({ length: 201 }, () => ({ id, revision: 1 })),
      }),
    ).toThrow();
    expect(() =>
      libraryCommandSchema.parse({ ...body, metadata: { title: "禁止混合" } }),
    ).toThrow();
  });
  it("旧回执按原body而非最新cache读取，重复、错scope及错动作均unknown", () => {
    const receipt = {
      current_actor_id: actor,
      current_org_id: org,
      library_id: library,
      scope,
      revision: 2,
      items: [
        {
          id,
          asset_id: null,
          folder_id: null,
          kind: "text",
          catalog_state: "active",
          trashed_at: null,
          revision: 2,
        },
      ],
    };
    expect(readLibraryReceipt(receipt, identity, body).revision).toBe(2);
    expect(() =>
      readLibraryReceipt({ ...receipt, revision: 5 }, identity, body),
    ).toThrow();
    expect(() =>
      readLibraryReceipt(
        { ...receipt, items: [...receipt.items, ...receipt.items] },
        identity,
        body,
      ),
    ).toThrow();
    expect(() =>
      readLibraryReceipt({ ...receipt, current_actor_id: id }, identity, body),
    ).toThrow();
    expect(() =>
      readLibraryReceipt(
        {
          ...receipt,
          items: [
            { ...receipt.items[0], catalog_state: "trashed", trashed_at: time },
          ],
        },
        identity,
        body,
      ),
    ).toThrow();
  });
});

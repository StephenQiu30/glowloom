import { describe, expect, it } from "vitest";
import {
  batchPrompt,
  reorderBatchColumns,
  swapBatchReferences,
  synchronizeBatchRows,
  type BatchTableConfig,
} from "./batch-table";

const id = (value: number) =>
  `00000000-0000-4000-8000-${String(value).padStart(12, "0")}`;
const table: BatchTableConfig = {
  version: 1,
  operation: "creative",
  globalPrompt: "",
  concurrency: 3,
  mode: "",
  outputCount: 1,
  modelProfileId: null,
  params: {},
  referenceColumns: [
    { id: id(1), label: "参考图 1" },
    { id: id(2), label: "参考图 2" },
  ],
  rows: [
    {
      id: id(3),
      prompt: "第一镜",
      enabled: true,
      inputNodeIds: [id(4), id(5)],
    },
    {
      id: id(6),
      prompt: "第二镜",
      enabled: false,
      inputNodeIds: [id(7), null],
    },
  ],
};

describe("迁移批量创作表算法", () => {
  it("重排参考列同时移动每行素材，保留空槽位", () => {
    const next = reorderBatchColumns(table, id(1), id(2));
    expect(next.referenceColumns.map((column) => column.id)).toEqual([
      id(2),
      id(1),
    ]);
    expect(next.rows[0].inputNodeIds).toEqual([id(5), id(4)]);
    expect(next.rows[1].inputNodeIds).toEqual([null, id(7)]);
    expect(table.rows[1].inputNodeIds).toEqual([id(7), null]);
  });
  it("跨行交换参考仅修改两处，拒绝越界及空源", () => {
    const next = swapBatchReferences(table, id(3), 1, id(6), 1);
    expect(next.rows[0].inputNodeIds).toEqual([id(4), null]);
    expect(next.rows[1].inputNodeIds).toEqual([id(7), id(5)]);
    expect(swapBatchReferences(table, id(6), 1, id(3), 1)).toBe(table);
    expect(swapBatchReferences(table, id(3), 1, id(6), 8)).toBe(table);
  });
  it("单例列广播、短列保留空槽，手工行与身份不丢失", () => {
    const next = synchronizeBatchRows(table, [[id(4), id(7)], [id(5)]], () =>
      id(10),
    );
    expect(next.rows).toEqual([
      { ...table.rows[0], inputNodeIds: [id(4), id(5)] },
      { ...table.rows[1], inputNodeIds: [id(7), id(5)] },
    ]);
    const changed = synchronizeBatchRows(table, [[id(4)], []], () => id(10));
    expect(changed.rows[0].inputNodeIds).toEqual([id(4), null]);
    expect(changed.rows[1]).toEqual(table.rows[1]);
  });
  it("全局提示词只在非空时覆盖行提示词", () => {
    expect(batchPrompt(table, table.rows[0])).toBe("第一镜");
    expect(
      batchPrompt({ ...table, globalPrompt: "  所有镜头  " }, table.rows[0]),
    ).toBe("所有镜头");
  });
});

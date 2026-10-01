import { describe, expect, it } from "vitest";
import {
  batchGenerationItems,
  batchGenerationGroups,
  mergeBatchQuotes,
} from "./batch-generation";
import { createBatchTable, createBatchRow } from "./batch-table";
import { createNode } from "./document";
import { CanvasNodeType } from "./model";
const model = {
  key: "own-image",
  capability: "image.edit",
  modes: ["edit"],
  inputRoles: ["prompt", "reference_image"],
};
function fixture() {
  const config = createBatchTable();
  config.mode = "edit";
  config.globalPrompt = " 全局描述 ";
  const images = [0, 1].map((index) =>
    createNode(
      CanvasNodeType.Image,
      { x: index * 400, y: 0 },
      crypto.randomUUID(),
      { assetId: crypto.randomUUID() },
    ),
  );
  const row = createBatchRow(config);
  row.inputNodeIds = [images[1].id, null, images[0].id];
  config.rows = [row];
  return { config, images, row };
}
describe("批量报价编排", () => {
  it("按非空参考列顺序映射稳定资产与目录角色，不将节点ID当媒体ID", () => {
    const { config, images } = fixture();
    const result = batchGenerationItems(config, images, model);
    expect(result.items[0].media_inputs).toEqual([
      { role: "reference_image", media_asset_id: images[1].assetId },
      { role: "reference_image", media_asset_id: images[0].assetId },
    ]);
    expect(result.items[0].prompt).toBe("全局描述");
  });
  it("重试指定启用行只产生这一行报价，并显式请求重新生成", () => {
    const { config, images, row } = fixture();
    const second = createBatchRow(config);
    second.inputNodeIds = [...row.inputNodeIds];
    config.rows.push(second);
    const result = batchGenerationItems(config, images, model, 0, row.id);
    expect(result.rowIds).toEqual([row.id]);
    expect(result.items[0].force_regenerate).toBe(true);
    row.enabled = false;
    expect(() =>
      batchGenerationItems(config, images, model, 0, row.id),
    ).toThrow();
  });
  it("最多150个启用行一个原子批次，丢失素材和无输入能力在报价前失败", () => {
    const { config, images, row } = fixture();
    config.rows = Array.from({ length: 151 }, () => ({
      ...row,
      id: crypto.randomUUID(),
      inputNodeIds: [...row.inputNodeIds],
    }));
    expect(batchGenerationItems(config, images, model).items).toHaveLength(150);
    expect(batchGenerationItems(config, images, model, 1).items).toHaveLength(
      1,
    );
    expect(() => batchGenerationItems(config, [], model)).toThrow();
    expect(() =>
      batchGenerationItems(config, images, {
        ...model,
        inputRoles: ["prompt"],
      }),
    ).toThrow();
  });
  it("500行全表按150分组，保留每一行身份且只排除关闭行", () => {
    const { config, images, row } = fixture();
    config.rows = Array.from({ length: 500 }, () => ({
      ...row,
      id: crypto.randomUUID(),
      inputNodeIds: [...row.inputNodeIds],
    }));
    const groups = batchGenerationGroups(config, images, model);
    expect(groups.map((group) => group.items.length)).toEqual([
      150, 150, 150, 50,
    ]);
    expect(groups.flatMap((group) => group.rowIds)).toEqual(
      config.rows.map((item) => item.id),
    );
    config.rows[3].enabled = false;
    expect(
      batchGenerationGroups(config, images, model).flatMap(
        (group) => group.rowIds,
      ),
    ).not.toContain(config.rows[3].id);
  });
  it("全表报价提前检查最后一组，防止前面报价后才发现遗漏行无效", () => {
    const { config, images, row } = fixture();
    config.globalPrompt = "";
    config.rows = Array.from({ length: 151 }, () => ({
      ...row,
      id: crypto.randomUUID(),
      inputNodeIds: [...row.inputNodeIds],
      prompt: "有效提示",
    }));
    config.rows[150].prompt = "";
    expect(() => batchGenerationGroups(config, images, model)).toThrow("151");
  });
  it("综合费用取最早到期和最小可用余额，拒绝重复操作身份和金额溢出", () => {
    const quote = (cost: number, available: number, expires: string) => ({
      batch_id: crypto.randomUUID(),
      items: [
        {
          operation_id: crypto.randomUUID(),
          target_label: "行",
          errors: [],
          quote_micros: cost,
        },
      ],
      total_micros: cost,
      available_micros: available,
      expires_at: expires,
      confirmable: true,
    });
    const groups = [
      quote(400, 900, "2026-10-01T14:00:00Z"),
      quote(300, 800, "2026-10-01T13:59:00Z"),
    ];
    expect(mergeBatchQuotes(groups)).toMatchObject({
      batch_id: null,
      total_micros: 700,
      available_micros: 800,
      expires_at: "2026-10-01T13:59:00Z",
      confirmable: true,
    });
    expect(
      mergeBatchQuotes([groups[0], { ...groups[1], available_micros: 500 }])
        .confirmable,
    ).toBe(false);
    expect(() => mergeBatchQuotes([groups[0], groups[0]])).toThrow();
    expect(() =>
      mergeBatchQuotes([
        quote(
          Number.MAX_SAFE_INTEGER,
          Number.MAX_SAFE_INTEGER,
          groups[0].expires_at,
        ),
        groups[1],
      ]),
    ).toThrow();
  });
});

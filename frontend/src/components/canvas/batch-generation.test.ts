import { describe, expect, it } from "vitest";
import { batchGenerationItems } from "./batch-generation";
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
});

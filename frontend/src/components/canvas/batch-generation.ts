import type { GenerationItem } from "@/components/operation/queries";
import type { QuoteResponse } from "@/components/operation/use-quote";
import {
  batchPrompt,
  batchTableSchema,
  type BatchTableConfig,
} from "./batch-table";
import { CanvasNodeType, type CanvasNodeData } from "./model";

export function batchGenerationItems(
  config: BatchTableConfig,
  nodes: CanvasNodeData[],
  model: {
    key: string;
    capability: string;
    modes: string[];
    inputRoles?: string[];
  },
  page = 0,
  rowId?: string,
): { items: GenerationItem[]; labels: string[]; rowIds: string[] } {
  const value = batchTableSchema.parse(config);
  if (!model.modes.includes(value.mode))
    throw new Error("请选择模型支持的生成方式。");
  const images = new Map(
    nodes
      .filter((node) => node.type === CanvasNodeType.Image && node.assetId)
      .map((node) => [node.id, node.assetId!]),
  );
  const enabled = value.rows
    .map((row, index) => ({ row, index }))
    .filter(({ row }) => row.enabled && (!rowId || row.id === rowId));
  const rows = rowId ? enabled : enabled.slice(page * 150, (page + 1) * 150);
  const roles = model.inputRoles ?? ["reference"];
  const role = roles.includes("reference")
    ? "reference"
    : roles.find((role) => role !== "prompt");
  if (!rows.length) throw new Error("此批没有启用的行。");
  return {
    labels: rows.map(({ index }) => `批量第 ${index + 1} 行`),
    rowIds: rows.map(({ row }) => row.id),
    items: rows.map(({ row, index }) => {
      const prompt = batchPrompt(value, row).trim();
      if (!prompt) throw new Error(`第 ${index + 1} 行需要提示词。`);
      if (
        value.operation === "try_on" &&
        (!row.inputNodeIds[0] || !row.inputNodeIds[1])
      )
        throw new Error(`第 ${index + 1} 行换装需要人物与服装两张参考图。`);
      const media_inputs = row.inputNodeIds.flatMap((id) => {
        if (!id) return [];
        const asset = images.get(id);
        if (!asset) throw new Error(`第 ${index + 1} 行包含已移除的参考素材。`);
        if (!role) throw new Error("所选模型不支持图片参考输入。");
        return [{ role, media_asset_id: asset }];
      });
      return {
        model_key: model.key,
        capability: model.capability,
        mode: value.mode,
        prompt,
        params: structuredClone(value.params),
        output_count: value.outputCount,
        force_regenerate: Boolean(rowId),
        media_inputs,
      };
    }),
  };
}

/** Prepare every enabled row before the first API call; each quote stays within REST limits. */
export function batchGenerationGroups(
  config: BatchTableConfig,
  nodes: CanvasNodeData[],
  model: Parameters<typeof batchGenerationItems>[2],
  rowId?: string,
) {
  const value = batchTableSchema.parse(config);
  const count = value.rows.filter(
    (row) => row.enabled && (!rowId || row.id === rowId),
  ).length;
  if (!count) throw new Error("没有启用的行。");
  return Array.from({ length: Math.ceil(count / 150) }, (_, page) =>
    batchGenerationItems(value, nodes, model, page, rowId),
  );
}

/** UI summary only: confirmation must still target the original server batch identities. */
export function mergeBatchQuotes(groups: QuoteResponse[]): QuoteResponse {
  if (!groups.length || groups.length > 4)
    throw new Error("报价分组数量无效。");
  const items = groups.flatMap((group) => group.items);
  const ids = items.flatMap((item) =>
    item.operation_id ? [item.operation_id] : [],
  );
  const total = groups.reduce((sum, group) => sum + group.total_micros, 0);
  const available = Math.min(...groups.map((group) => group.available_micros));
  const expiry = Math.min(
    ...groups.map((group) => Date.parse(group.expires_at)),
  );
  if (
    ids.length !== new Set(ids).size ||
    items.length > 500 ||
    ![total, available, ...groups.map((group) => group.total_micros)].every(
      (value) => Number.isSafeInteger(value) && value >= 0,
    ) ||
    !Number.isFinite(expiry)
  )
    throw new Error("报价身份或金额无效，请重新读取。");
  return {
    batch_id: null,
    items,
    expires_at: groups.find((group) => Date.parse(group.expires_at) === expiry)!
      .expires_at,
    total_micros: total,
    available_micros: available,
    confirmable:
      groups.every((group) => group.confirmable) && total <= available,
  };
}

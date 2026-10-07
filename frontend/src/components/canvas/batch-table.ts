// Derived from BeefTV 1ae25027:web/src/lib/canvas/canvas-batch-table.ts.
// MIT; see workspace/content/licenses/beeftv.txt. UUID identities and nullable reference slots
// adapt the source interaction to Lanverse's document/command contract.
import { z } from "zod";

export const batchTableSchema = z
  .object({
    version: z.literal(1),
    operation: z.enum(["try_on", "creative"]),
    globalPrompt: z.string().max(10000),
    concurrency: z.number().int().min(1).max(32),
    mode: z.string().max(128),
    outputCount: z.number().int().min(1).max(8),
    modelProfileId: z.string().uuid().nullable(),
    params: z.record(z.string(), z.json()),
    referenceColumns: z
      .array(
        z
          .object({ id: z.string().uuid(), label: z.string().min(1).max(128) })
          .strict(),
      )
      .min(1)
      .max(6),
    rows: z
      .array(
        z
          .object({
            id: z.string().uuid(),
            enabled: z.boolean(),
            prompt: z.string().max(10000),
            inputNodeIds: z.array(z.string().uuid().nullable()).min(1).max(6),
          })
          .strict(),
      )
      .max(500),
  })
  .strict()
  .superRefine((value, context) => {
    if (
      new Set(value.referenceColumns.map((column) => column.id)).size !==
      value.referenceColumns.length
    )
      context.addIssue({ code: "custom", message: "参考列身份重复。" });
    if (new Set(value.rows.map((row) => row.id)).size !== value.rows.length)
      context.addIssue({ code: "custom", message: "批量行身份重复。" });
    if (
      value.rows.some(
        (row) => row.inputNodeIds.length !== value.referenceColumns.length,
      )
    )
      context.addIssue({
        code: "custom",
        message: "参考素材必须与列一一对应。",
      });
  });
export type BatchTableConfig = z.infer<typeof batchTableSchema>;
export type BatchRow = BatchTableConfig["rows"][number];
const defaultPrompts = {
  try_on:
    "参考图1是人物原图，参考图2是目标服装。保持人物身份、五官、姿态和背景不变，将人物服装替换为参考图2中的款式，光影与原图一致。",
  creative:
    "基于参考图创作新图片，保留主体身份和关键细节，构图完整，光影自然。",
};
export function createBatchTable(): BatchTableConfig {
  return {
    version: 1,
    operation: "creative",
    globalPrompt: "",
    concurrency: 3,
    mode: "",
    outputCount: 1,
    modelProfileId: null,
    params: {},
    referenceColumns: Array.from({ length: 3 }, (_, index) => ({
      id: crypto.randomUUID(),
      label: `参考图 ${index + 1}`,
    })),
    rows: [],
  };
}
export function createBatchRow(
  table: BatchTableConfig,
  id = crypto.randomUUID(),
): BatchRow {
  return {
    id,
    enabled: true,
    prompt: defaultPrompts[table.operation],
    inputNodeIds: table.referenceColumns.map(
      (_, index) => table.rows.at(-1)?.inputNodeIds[index] ?? null,
    ),
  };
}
export function batchPrompt(table: BatchTableConfig, row: BatchRow) {
  return table.globalPrompt.trim() || row.prompt;
}
export function reorderBatchColumns(
  table: BatchTableConfig,
  fromId: string,
  toId: string,
): BatchTableConfig {
  const from = table.referenceColumns.findIndex(
    (column) => column.id === fromId,
  );
  const to = table.referenceColumns.findIndex((column) => column.id === toId);
  if (from < 0 || to < 0 || from === to) return table;
  const referenceColumns = [...table.referenceColumns];
  const [moved] = referenceColumns.splice(from, 1);
  referenceColumns.splice(to, 0, moved);
  return {
    ...table,
    referenceColumns: referenceColumns.map((column, index) => ({
      ...column,
      label: `参考图 ${index + 1}`,
    })),
    rows: table.rows.map((row) => ({
      ...row,
      inputNodeIds: referenceColumns.map(
        (column) =>
          row.inputNodeIds[
            table.referenceColumns.findIndex((value) => value.id === column.id)
          ] ?? null,
      ),
    })),
  };
}
export function swapBatchReferences(
  table: BatchTableConfig,
  sourceId: string,
  sourceColumn: number,
  targetId: string,
  targetColumn: number,
): BatchTableConfig {
  if (sourceId === targetId && sourceColumn === targetColumn) return table;
  if (
    ![sourceColumn, targetColumn].every(
      (value) =>
        Number.isInteger(value) &&
        value >= 0 &&
        value < table.referenceColumns.length,
    )
  )
    return table;
  const source = table.rows.find((row) => row.id === sourceId),
    target = table.rows.find((row) => row.id === targetId);
  if (!source || !target || !source.inputNodeIds[sourceColumn]) return table;
  const sourceReference = source.inputNodeIds[sourceColumn],
    targetReference = target.inputNodeIds[targetColumn];
  return {
    ...table,
    rows: table.rows.map((row) => {
      if (row.id !== sourceId && row.id !== targetId) return row;
      const inputNodeIds = [...row.inputNodeIds];
      if (row.id === sourceId)
        inputNodeIds[sourceColumn] = targetReference ?? null;
      if (row.id === targetId) inputNodeIds[targetColumn] = sourceReference;
      return { ...row, inputNodeIds };
    }),
  };
}
export function synchronizeBatchRows(
  table: BatchTableConfig,
  columns: string[][],
  newId: () => string = () => crypto.randomUUID(),
): BatchTableConfig {
  if (columns.length !== table.referenceColumns.length)
    throw new Error("参考列数量与表格不一致。");
  const count = Math.max(0, ...columns.map((column) => column.length));
  const unmatched = [...table.rows];
  const rows = Array.from({ length: count }, (_, index) => {
    const inputNodeIds = columns.map(
      (column) => (column.length === 1 ? column[0] : column[index]) ?? null,
    );
    let previousIndex = unmatched.findIndex((row) =>
      row.inputNodeIds.every((value, column) => value === inputNodeIds[column]),
    );
    if (previousIndex < 0 && inputNodeIds[0])
      previousIndex = unmatched.findIndex(
        (row) => row.inputNodeIds[0] === inputNodeIds[0],
      );
    const previous =
      previousIndex >= 0 ? unmatched.splice(previousIndex, 1)[0] : undefined;
    return { ...(previous ?? createBatchRow(table, newId())), inputNodeIds };
  });
  const result = { ...table, rows: [...rows, ...unmatched] };
  return batchTableSchema.parse(result);
}

export function encodeBatchConfig(table: BatchTableConfig) {
  const value = batchTableSchema.parse(table);
  return {
    version: value.version,
    operation: value.operation,
    global_prompt: value.globalPrompt,
    concurrency: value.concurrency,
    mode: value.mode,
    output_count: value.outputCount,
    model_profile_id: value.modelProfileId,
    params: value.params,
    reference_columns: value.referenceColumns,
    rows: value.rows.map((row) => ({
      id: row.id,
      enabled: row.enabled,
      prompt: row.prompt,
      input_node_ids: row.inputNodeIds,
    })),
  };
}
export const batchWireSchema = z
  .object({
    version: z.literal(1),
    operation: z.enum(["try_on", "creative"]),
    global_prompt: z.string(),
    concurrency: z.number(),
    mode: z.string(),
    output_count: z.number(),
    model_profile_id: z.string().uuid().nullable(),
    params: z.record(z.string(), z.json()),
    reference_columns: z.array(
      z.object({ id: z.string(), label: z.string() }).strict(),
    ),
    rows: z.array(
      z
        .object({
          id: z.string(),
          enabled: z.boolean(),
          prompt: z.string(),
          input_node_ids: z.array(z.string().nullable()),
        })
        .strict(),
    ),
  })
  .strict();
export function decodeBatchConfig(value: unknown): BatchTableConfig {
  const wire = batchWireSchema.parse(value);
  return batchTableSchema.parse({
    version: wire.version,
    operation: wire.operation,
    globalPrompt: wire.global_prompt,
    concurrency: wire.concurrency,
    mode: wire.mode,
    outputCount: wire.output_count,
    modelProfileId: wire.model_profile_id,
    params: wire.params,
    referenceColumns: wire.reference_columns,
    rows: wire.rows.map((row) => ({
      id: row.id,
      enabled: row.enabled,
      prompt: row.prompt,
      inputNodeIds: row.input_node_ids,
    })),
  });
}

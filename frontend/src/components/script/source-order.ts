import { z } from "zod";
import { scriptUUID } from "./source-model";
const sourceOrder = z
  .array(scriptUUID)
  .refine((ids) => new Set(ids).size === ids.length);
export function reviewSourceOrder(
  previous: readonly string[],
  latest: readonly string[],
) {
  sourceOrder.parse(previous);
  sourceOrder.parse(latest);
  const existing = new Set(previous);
  const current = new Set(latest);
  const added = latest.filter((id) => !existing.has(id));
  const removed = previous.filter((id) => !current.has(id));
  return {
    order: [...previous.filter((id) => current.has(id)), ...added],
    added,
    removed,
  };
}
export function moveSourcePosition(
  order: readonly string[],
  lineageId: string,
  position: number,
) {
  sourceOrder.parse(order);
  const previous = order.indexOf(lineageId);
  if (
    previous < 0 ||
    !Number.isInteger(position) ||
    position < 0 ||
    position >= order.length
  )
    throw new Error("来源位置或完整身份集合无效。");
  const next = [...order];
  next.splice(previous, 1);
  next.splice(position, 0, lineageId);
  return next;
}

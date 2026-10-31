"use client";
import { isProjectID } from "@/components/projects/project";
import { createContext } from "react";
import type { CanvasNodeData } from "../types/canvas";
export type CanvasMediaSelection = (
  target: string,
  kinds: string[],
  signal: AbortSignal,
) => Promise<CanvasNodeData | null>;
/** A transient canvas gesture. This never creates a connection or saves a source. */
export const CanvasMediaSelectionContext =
  createContext<CanvasMediaSelection | null>(null);

export function isCanvasMediaCandidate(
  node: CanvasNodeData,
  target: string,
  kinds: string[],
) {
  return (
    node.id !== target &&
    kinds.includes(node.type) &&
    isProjectID(node.serviceData?.media_ref?.asset_id)
  );
}

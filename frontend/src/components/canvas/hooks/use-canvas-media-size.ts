"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import type { CanvasNodeData } from "../types/canvas";
import { fitNodeSize } from "../lib/canvas-node-size";

export type CanvasMediaDimensions = { width: number; height: number };
type Measurement = CanvasMediaDimensions & { source: string };

/** Size is business geometry; the decoded dimensions and preview URL remain local. */
export function fitCanvasMediaNode(
  node: CanvasNodeData,
  measurement: Measurement,
  baseUrl: string,
): CanvasNodeData {
  if (
    !["image", "video"].includes(node.type) ||
    !node.metadata?.content ||
    !Number.isFinite(measurement.width) ||
    !Number.isFinite(measurement.height) ||
    measurement.width <= 0 ||
    measurement.height <= 0
  )
    return node;
  try {
    if (
      new URL(node.metadata.content, baseUrl).href !==
      new URL(measurement.source, baseUrl).href
    )
      return node;
  } catch {
    return node;
  }
  const ratio = measurement.width / measurement.height;
  const size =
    node.metadata.freeResize || Math.abs(node.width - node.height * ratio) < 0.5
      ? { width: node.width, height: node.height }
      : fitNodeSize(
          measurement.width,
          measurement.height,
          node.width,
          node.height,
        );
  if (
    node.width === size.width &&
    node.height === size.height &&
    node.metadata.naturalWidth === measurement.width &&
    node.metadata.naturalHeight === measurement.height
  )
    return node;
  return {
    ...node,
    ...size,
    position: {
      x: node.position.x + (node.width - size.width) / 2,
      y: node.position.y + (node.height - size.height) / 2,
    },
    metadata: {
      ...node.metadata,
      naturalWidth: measurement.width,
      naturalHeight: measurement.height,
    },
  };
}

/** Defer late media decoding until the current gesture/IME finishes. */
export function useCanvasMediaSize(
  nodes: CanvasNodeData[],
  enabled: boolean,
  setNodes: Dispatch<SetStateAction<CanvasNodeData[]>>,
) {
  const [measurements, setMeasurements] = useState<Record<string, Measurement>>(
    {},
  );
  const applied = useRef<Record<string, Measurement>>({});
  const measure = useCallback(
    (id: string, source: string, dimensions?: CanvasMediaDimensions) => {
      if (!dimensions) return;
      setMeasurements((previous) => {
        const alive = new Set(nodes.map((node) => node.id));
        const retained = Object.fromEntries(
          Object.entries(previous).filter(([key]) => alive.has(key)),
        );
        return { ...retained, [id]: { source, ...dimensions } };
      });
    },
    [nodes],
  );
  useEffect(() => {
    if (!enabled) return;
    const pending = Object.fromEntries(
      Object.entries(measurements).filter(
        ([id, value]) => applied.current[id] !== value,
      ),
    );
    if (!Object.keys(pending).length) return;
    applied.current = measurements;
    setNodes((previous) => {
      let changed = false;
      const next = previous.map((node) => {
        const measurement = pending[node.id];
        const fitted = measurement
          ? fitCanvasMediaNode(node, measurement, document.baseURI)
          : node;
        changed ||= fitted !== node;
        return fitted;
      });
      return changed ? next : previous;
    });
  }, [nodes, measurements, enabled, setNodes]);
  return measure;
}

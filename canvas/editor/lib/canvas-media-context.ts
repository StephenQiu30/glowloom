"use client";
import { createContext } from "react";
import type { CanvasFilePreviewContext } from "@glowloom/canvas-host/components/canvas/lib/canvas-file-preview";

export const CanvasMediaContext = createContext<
  CanvasFilePreviewContext | undefined
>(undefined);

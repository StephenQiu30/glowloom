"use client";
import { createContext } from "react";
import type { CanvasFilePreviewContext } from "@/components/canvas/lib/canvas-file-preview";

export const CanvasMediaContext = createContext<
  CanvasFilePreviewContext | undefined
>(undefined);

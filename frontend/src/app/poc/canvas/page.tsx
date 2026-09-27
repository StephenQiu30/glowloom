import type { Metadata } from "next";

import { PocCanvas } from "@/features/canvas/poc-canvas";

export const metadata: Metadata = {
  title: "画布性能 PoC | Lanverse",
  robots: { index: false, follow: false },
};

export default function CanvasPocPage() {
  return <PocCanvas />;
}

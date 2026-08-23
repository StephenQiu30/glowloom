import type { Metadata } from "next";

import { ComicProductionStudio } from "./comic-production-studio";

export const metadata: Metadata = {
  title: "资产库 · Lanverse",
  description: "AI 漫剧角色资产、一致性与生产影响管理工作台",
};

export default function StudioPage() {
  return <ComicProductionStudio />;
}

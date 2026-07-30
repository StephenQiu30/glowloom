import type { Metadata } from "next";

import { EpisodeProductionStudio } from "../episode-production-studio";

export const metadata: Metadata = {
  title: "剧本结构 · Lanverse",
  description: "AI 漫剧剧本版本、结构提取与人工决议工作台",
};

export default async function EpisodeScriptPage({
  params,
}: {
  params: Promise<{ episodeId: string }>;
}) {
  const { episodeId } = await params;
  return <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />;
}

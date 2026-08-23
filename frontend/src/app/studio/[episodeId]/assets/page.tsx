import type { Metadata } from "next";

import { EpisodeProductionStudio } from "../episode-production-studio";

export const metadata: Metadata = {
  title: "资产准备 · Lanverse",
  description: "AI 漫剧单集角色、场景与声音资产准备度",
};

export default async function EpisodeAssetsPage({
  params,
}: {
  params: Promise<{ episodeId: string }>;
}) {
  const { episodeId } = await params;
  return <EpisodeProductionStudio episodeId={episodeId} initialPanel="assets" />;
}

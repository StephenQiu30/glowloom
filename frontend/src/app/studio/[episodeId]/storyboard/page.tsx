import type { Metadata } from "next";

import { EpisodeProductionStudio } from "../episode-production-studio";

export const metadata: Metadata = {
  title: "分镜设计 · Lanverse",
  description: "AI 漫剧镜头清单、完整规格版本与生产准备度",
};

export default async function EpisodeStoryboardPage({
  params,
}: {
  params: Promise<{ episodeId: string }>;
}) {
  const { episodeId } = await params;
  return <EpisodeProductionStudio episodeId={episodeId} initialPanel="storyboard" />;
}

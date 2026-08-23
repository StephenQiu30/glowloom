import type { Metadata } from "next";

import { EpisodeProductionStudio } from "../episode-production-studio";

export const metadata: Metadata = {
  title: "媒体 · Lanverse",
  description: "AI 漫剧私有媒体上传、探测与稳定版本",
};

export default async function EpisodeMediaPage({
  params,
}: {
  params: Promise<{ episodeId: string }>;
}) {
  const { episodeId } = await params;
  return <EpisodeProductionStudio episodeId={episodeId} initialPanel="media" />;
}

/** Resolve display references only within the proposal's frozen source and run. */
export function proposalContext(proposal: API.CreationProposal, proposals: API.CreationProposal[]) {
  const related = proposals.filter((item) => item.run_id === proposal.run_id && item.source_revision_id === proposal.source_revision_id && item.source_hash === proposal.source_hash);
  const episodes = (related.find((item) => item.stage === "map_manuscript")?.candidate as API.CreationTextEpisodeMap | undefined)?.episodes ?? [];
  const analyses = related.filter((item) => item.stage === "analyze_episode").map((item) => item.candidate as API.CreationTextEpisodeAnalysis);
  function episodeLabel(key: string) {
    const index = episodes.findIndex((item) => item.key === key);
    const episode = episodes[index];
    return episode ? `第 ${episode.number ?? index + 1} 集 · ${episode.title}` : `剧集 ${key}`;
  }
  function scene(episodeKey: string, sceneKey: string) {
    return analyses.find((item) => item.episode_key === episodeKey)?.scenes.find((item) => item.key === sceneKey);
  }
  function location(episodeKey: string, sceneKey: string) {
    return `${episodeLabel(episodeKey)} / ${scene(episodeKey, sceneKey)?.title ?? `场景 ${sceneKey}`}`;
  }
  return { episodeLabel, scene, location };
}

export function proposalTitle(proposal: API.CreationProposal, proposals: API.CreationProposal[]) {
  const context = proposalContext(proposal, proposals);
  if (proposal.stage === "map_manuscript") return "全剧分集方案";
  if (proposal.stage === "build_world") return "人物、场景与道具";
  if (proposal.stage === "analyze_episode") return context.episodeLabel((proposal.candidate as API.CreationTextEpisodeAnalysis).episode_key);
  const candidate = proposal.candidate as API.CreationTextSceneDirection;
  return context.location(candidate.episode_key, candidate.scene_key);
}

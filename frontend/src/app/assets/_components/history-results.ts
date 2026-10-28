import type {
  AssetKind,
  HistoryOutput,
  PersonalHistoryItem,
} from "./asset-query";

export type HistoryCardTarget = {
  key: string;
  task: PersonalHistoryItem;
  output?: HistoryOutput;
  outputs: HistoryOutput[];
  favorite: boolean;
  canFavorite: boolean;
  canDownload: boolean;
  canPublish: boolean;
};

export function resultTargetKey(operation: string, output?: string | null) {
  return `${operation}:${output ?? "record"}`;
}

export function historyCardTargets(
  items: PersonalHistoryItem[],
  kind: AssetKind,
): HistoryCardTarget[] {
  return items.flatMap((task) => {
    const outputs = task.outputs;
    const candidates =
      kind === "video" || !outputs.length ? [undefined] : outputs;
    return candidates.map((output) => {
      const results = output ? [output] : outputs;
      const passed =
        results.length > 0 &&
        results.every((item) => item.moderation_status === "passed");
      const downloadable =
        task.status === "completed" &&
        passed &&
        results.every((item) => item.can_download === true);
      const dubbing =
        task.capability === "video.dubbing" ||
        ["video_dubbing", "dubbing"].includes(task.mode ?? "");
      return {
        key: resultTargetKey(task.id, output?.id),
        task,
        output,
        outputs: results,
        favorite: results.length > 0 && results.every((item) => item.favorite),
        canFavorite: results.length > 0,
        canDownload: downloadable,
        canPublish:
          task.status === "completed" &&
          passed &&
          (kind === "image" || kind === "video") &&
          !dubbing &&
          results.every(
            (item) => item.can_publish === true && !!item.media_asset_id,
          ),
      };
    });
  });
}

export function historyCommandTarget(card: HistoryCardTarget) {
  return {
    operation_id: card.task.id,
    output_id: card.output?.id,
    expected_revision:
      card.output?.metadata_revision ?? card.task.history_revision,
  };
}

export function historyPublicationTargets(cards: HistoryCardTarget[]) {
  return cards.flatMap((card) =>
    card.outputs.map((output) => ({
      operation_id: card.task.id,
      output_id: output.id,
    })),
  );
}

export function isVideoDubbing(card: HistoryCardTarget) {
  return (
    card.task.capability === "video.dubbing" ||
    ["video_dubbing", "dubbing"].includes(card.task.mode ?? "")
  );
}

export function canBatchDownload(cards: HistoryCardTarget[], kind: AssetKind) {
  if (!cards.length || cards.some((card) => !card.canDownload)) return false;
  return (
    kind !== "video" ||
    cards.length === 1 ||
    cards.every(
      (card) =>
        !(
          [
            "VideoBGM",
            "AudioVideoMix",
            "video_bgm",
            "audio_video_mix",
          ].includes(card.task.mode ?? "") ||
          ["video.bgm", "audio.video_mix"].includes(card.task.capability ?? "")
        ),
    )
  );
}

export function hasOriginalDocument(card: HistoryCardTarget) {
  return (
    card.canDownload &&
    card.outputs.length > 0 &&
    card.outputs.every(
      (output) => output.kind === "document" && !!output.media_asset_id,
    )
  );
}

export function hasDocumentBody(card: HistoryCardTarget) {
  return (
    card.canDownload &&
    card.outputs.length > 0 &&
    card.outputs.every(
      (output) =>
        ["json", "text"].includes(output.kind ?? "") && !output.media_asset_id,
    )
  );
}

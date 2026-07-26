"use client";

import { useState } from "react";

import { newIntentKey } from "@/features/intent-key";
import { StoryView, type StoryVersionItem } from "@/features/story/story-view";
import {
  useConfirmScriptMutation,
  useConfirmSourceMutation,
  useConfirmStoryboardMutation,
  useCreateSourceRevisionMutation,
  useGenerateScriptMutation,
  useGenerateStoryboardMutation,
  useListScriptVersionsQuery,
  useListSourceRevisionsQuery,
  useListStoryboardVersionsQuery,
} from "@/store/backend-api";

interface StoryWorkspaceProps {
  episodeId: string;
}

function versions(
  items: readonly { id: string; version: number; status: string; resource_version: number }[],
): StoryVersionItem[] {
  return items.map((item) => ({
    id: item.id,
    version: item.version,
    status: item.status as StoryVersionItem["status"],
    resourceVersion: item.resource_version,
  }));
}

function isConflict(error: unknown): boolean {
  return typeof error === "object" && error !== null && "status" in error && error.status === 412;
}

export function StoryWorkspace({ episodeId }: StoryWorkspaceProps) {
  const sources = useListSourceRevisionsQuery(episodeId);
  const scripts = useListScriptVersionsQuery(episodeId);
  const storyboards = useListStoryboardVersionsQuery(episodeId);
  const [createSource] = useCreateSourceRevisionMutation();
  const [confirmSource] = useConfirmSourceMutation();
  const [generateScript] = useGenerateScriptMutation();
  const [confirmScript] = useConfirmScriptMutation();
  const [generateStoryboard] = useGenerateStoryboardMutation();
  const [confirmStoryboard] = useConfirmStoryboardMutation();
  const [conflictStage, setConflictStage] = useState<string>();
  const [actionError, setActionError] = useState<string>();

  async function versionAction(stage: string, action: () => Promise<unknown>) {
    setConflictStage(undefined);
    setActionError(undefined);
    try {
      await action();
    } catch (error) {
      if (isConflict(error)) {
        setConflictStage(stage);
      } else {
        setActionError("STORY_ACTION_FAILED");
      }
    }
  }

  const sourceItems = sources.data?.items ?? [];
  return (
    <StoryView
      conflictStage={conflictStage}
      error={
        sources.error || scripts.error || storyboards.error ? "STORY_UNAVAILABLE" : actionError
      }
      loading={sources.isLoading || scripts.isLoading || storyboards.isLoading}
      onConfirmScript={(versionId, resourceVersion) =>
        void versionAction("剧本", () => confirmScript({ versionId, resourceVersion }).unwrap())
      }
      onConfirmSource={(versionId, resourceVersion) =>
        void versionAction("来源", () => confirmSource({ versionId, resourceVersion }).unwrap())
      }
      onConfirmStoryboard={(versionId, resourceVersion) =>
        void versionAction("分镜", () => confirmStoryboard({ versionId, resourceVersion }).unwrap())
      }
      onCreateSource={({ content, rights }) =>
        void versionAction("来源", () =>
          createSource({
            episodeId,
            content,
            rightsBasis: rights,
            parentId: sourceItems.find((item) => item.status === "confirmed")?.id ?? null,
            idempotencyKey: newIntentKey("create-source"),
          }).unwrap(),
        )
      }
      onGenerateScript={() =>
        void versionAction("剧本", () =>
          generateScript({ episodeId, idempotencyKey: newIntentKey("generate-script") }).unwrap(),
        )
      }
      onGenerateStoryboard={() =>
        void versionAction("分镜", () =>
          generateStoryboard({
            episodeId,
            idempotencyKey: newIntentKey("generate-board"),
          }).unwrap(),
        )
      }
      scripts={versions(scripts.data?.items ?? [])}
      sources={versions(sourceItems)}
      storyboards={versions(storyboards.data?.items ?? [])}
    />
  );
}

"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { newIntentKey } from "@/features/intent-key";
import { ProjectsView } from "@/features/projects/projects-view";
import { useCreateProjectMutation, useListProjectsQuery } from "@/store/backend-api";

export function ProjectsWorkspace() {
  const router = useRouter();
  const projects = useListProjectsQuery();
  const [createProject] = useCreateProjectMutation();
  const [creationError, setCreationError] = useState<string>();

  async function create(title: string) {
    setCreationError(undefined);
    try {
      const created = await createProject({
        title,
        idempotencyKey: newIntentKey("create-project"),
      }).unwrap();
      router.push(`/episodes/${created.episode.id}/story`);
    } catch {
      setCreationError("PROJECT_CREATE_FAILED");
    }
  }

  return (
    <ProjectsView
      error={projects.error ? "PROJECTS_UNAVAILABLE" : creationError}
      loading={projects.isLoading}
      onCreate={create}
      projects={(projects.data?.items ?? []).map((item) => ({
        id: item.project.id,
        title: item.project.title,
        episodeId: item.episode.id,
        hasConfirmedSource: item.episode.current_source_revision_id !== null,
      }))}
    />
  );
}

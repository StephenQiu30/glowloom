"use client";

import useSWRImmutable from "swr/immutable";
import { ensureIndependentCreation, getProject } from "@/api/projects";
import { useOptionalSession } from "@/components/auth/session";
import { requireProject } from "@/components/projects/project";
import { useSpace } from "@/components/spaces/space-context";

/** Ordinary chat belongs to the active space. Only explicit legacy links use a business project. */
export function useConversationScope(projectID?: string, enabled = true) {
  const session = useOptionalSession();
  const { selected } = useSpace();
  return useSWRImmutable(
    session && selected && enabled
      ? ([
          "conversation-scope",
          session.actor.id,
          session.session_id,
          selected.id,
          selected.writable,
          projectID ?? null,
        ] as const)
      : null,
    async ([, , , , , project]) =>
      requireProject(
        project
          ? await getProject({ pid: project })
          : await ensureIndependentCreation(),
      ),
    { shouldRetryOnError: false },
  );
}

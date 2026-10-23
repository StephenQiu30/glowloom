"use client";
import { useLayoutEffect, useRef } from "react";
import { useSWRConfig } from "swr";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import type { ProjectCommandTarget } from "../hooks/use-projects";
import { invalidateWorkspaceResource } from "../lib/workspace-commands";
import { WorkspaceResourceActionDialog } from "./workspace-resource-action-dialog";

/** Legacy canvas lists share the same editor as mixed project contents. */
export function RenameProjectDialog({
  project,
  onClose,
}: {
  project: ProjectCommandTarget;
  onClose: () => void;
}) {
  const { account, ready, signingOut } = useAccount();
  const identity =
    ready && account && !signingOut ? `USER:${account.id}` : null;
  const opening = useRef(identity);
  const live = useRef(identity);
  useLayoutEffect(() => {
    live.current = identity;
  }, [identity]);
  const { mutate } = useSWRConfig();
  return (
    <WorkspaceResourceActionDialog
      operation="rename"
      targets={[{ ...project, kind: "canvas" }]}
      onClose={onClose}
      isCurrentActor={() =>
        !!opening.current && opening.current === live.current
      }
      onChanged={(command) => {
        if (opening.current)
          invalidateWorkspaceResource(mutate, opening.current, command);
      }}
    />
  );
}

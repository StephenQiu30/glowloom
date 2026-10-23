"use client";

import {
  Activity,
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  useRef,
  useLayoutEffect,
  type ReactNode,
} from "react";
import { CreateProjectDialog } from "./create-project-dialog";
import { RenameProjectDialog } from "./rename-project-dialog";
import { CanvasDeleteProjectsDialog } from "./canvas-delete-projects-dialog";
import { BatchDeleteProjectsDialog } from "./batch-delete-projects-dialog";
import type { ProjectCommandTarget } from "../hooks/use-projects";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { useAuthDialog } from "@/components/canvas/runtime/components/account/auth/auth-dialog-provider";
import { WorkspaceResourceActionDialog } from "./workspace-resource-action-dialog";
import type { WorkspaceResourceTarget } from "../lib/workspace-resources";
import type { WorkspaceCommand } from "../lib/workspace-commands";

type ProjectAction = { actor: string } & (
  | {
      kind: "resource";
      operation: "rename" | "delete";
      targets: WorkspaceResourceTarget[];
      onDeleted?: (key: string) => void;
      onChanged?: (command: WorkspaceCommand) => void;
      returnFocus?: HTMLElement | null;
    }
  | { kind: "create"; returnFocus?: HTMLElement | null }
  | { kind: "rename"; project: ProjectCommandTarget }
  | { kind: "delete"; project: ProjectCommandTarget; onDeleted?: () => void }
  | {
      kind: "batch-delete";
      projects: ProjectCommandTarget[];
      onDeleted?: (id: string) => void;
    }
);
type ProjectActions = {
  active: boolean;
  openResourceAction: (
    operation: "rename" | "delete",
    targets: WorkspaceResourceTarget[],
    returnFocus?: HTMLElement | null,
    onDeleted?: (key: string) => void,
    onChanged?: (command: WorkspaceCommand) => void,
  ) => void;
  openCreate: (trigger?: { currentTarget: HTMLElement | null }) => void;
  openRename: (project: ProjectCommandTarget) => void;
  openDelete: (project: ProjectCommandTarget, onDeleted?: () => void) => void;
  openBatchDelete: (
    projects: ProjectCommandTarget[],
    onDeleted?: (id: string) => void,
  ) => void;
};
const ProjectActionContext = createContext<ProjectActions | null>(null);

function target(project: ProjectCommandTarget): ProjectCommandTarget {
  return {
    id: project.id,
    title: project.title,
    coverFileId: project.coverFileId,
    coverFileVersion: project.coverFileVersion,
    revision: project.revision,
    projectBindingRevision: project.projectBindingRevision,
  };
}

// Route changes must not discard an unresolved command or its recovery dialog.
export function ProjectActionProvider({ children }: { children: ReactNode }) {
  const { account, ready, signingOut } = useAccount();
  const openAuth = useAuthDialog();
  const actor = ready && account && !signingOut ? String(account.id) : null;
  const liveActor = useRef(actor);
  useLayoutEffect(() => {
    liveActor.current = actor;
  }, [actor]);
  const [action, setAction] = useState<ProjectAction | null>(null);
  const openResourceAction = useCallback(
    (
      operation: "rename" | "delete",
      targets: WorkspaceResourceTarget[],
      returnFocus?: HTMLElement | null,
      onDeleted?: (key: string) => void,
      onChanged?: (command: WorkspaceCommand) => void,
    ) => {
      if (!actor) {
        openAuth();
        return;
      }
      if (!targets.length) return;
      setAction(
        (current) =>
          current ?? {
            kind: "resource",
            actor,
            operation,
            targets: targets.map((target) => ({ ...target })),
            returnFocus,
            onDeleted,
            onChanged,
          },
      );
    },
    [actor, openAuth],
  );
  const openCreate = useCallback(
    (trigger?: { currentTarget: HTMLElement | null }) => {
      if (!ready || !account || signingOut) {
        openAuth();
        return;
      }
      const returnFocus = trigger?.currentTarget;
      setAction(
        (current) =>
          current ?? {
            kind: "create",
            actor: String(account.id),
            returnFocus,
          },
      );
    },
    [ready, account, signingOut, openAuth],
  );
  const openRename = useCallback(
    (project: ProjectCommandTarget) => {
      if (!actor) {
        openAuth();
        return;
      }
      setAction(
        (current) =>
          current ?? { kind: "rename", actor, project: target(project) },
      );
    },
    [actor, openAuth],
  );
  const openDelete = useCallback(
    (project: ProjectCommandTarget, onDeleted?: () => void) => {
      if (!actor) {
        openAuth();
        return;
      }
      setAction(
        (current) =>
          current ?? {
            kind: "delete",
            actor,
            project: target(project),
            onDeleted,
          },
      );
    },
    [actor, openAuth],
  );
  const openBatchDelete = useCallback(
    (projects: ProjectCommandTarget[], onDeleted?: (id: string) => void) => {
      if (!actor) {
        openAuth();
        return;
      }
      if (!projects.length) return;
      setAction(
        (current) =>
          current ?? {
            kind: "batch-delete",
            actor,
            projects: projects.map(target),
            onDeleted,
          },
      );
    },
    [actor, openAuth],
  );
  const close = () => setAction(null);
  const value = useMemo(
    () => ({
      active: action !== null,
      openCreate,
      openRename,
      openDelete,
      openBatchDelete,
      openResourceAction,
    }),
    [
      action,
      openCreate,
      openRename,
      openDelete,
      openBatchDelete,
      openResourceAction,
    ],
  );
  return (
    <ProjectActionContext.Provider value={value}>
      {children}
      <Activity mode={!action || action.actor === actor ? "visible" : "hidden"}>
        {action?.kind === "resource" && (
          <WorkspaceResourceActionDialog
            operation={action.operation}
            targets={action.targets}
            onClose={close}
            onDeleted={action.onDeleted}
            onChanged={action.onChanged}
            returnFocus={action.returnFocus}
            isCurrentActor={() => liveActor.current === action.actor}
          />
        )}
        {action?.kind === "create" && (
          <CreateProjectDialog
            returnFocus={action.returnFocus}
            open
            onOpenChange={(open) => {
              if (!open) close();
            }}
          />
        )}
        {action?.kind === "rename" && (
          <RenameProjectDialog project={action.project} onClose={close} />
        )}
        {action?.kind === "delete" && (
          <CanvasDeleteProjectsDialog
            project={action.project}
            onClose={close}
            onDeleted={action.onDeleted}
          />
        )}
        {action?.kind === "batch-delete" && (
          <BatchDeleteProjectsDialog
            projects={action.projects}
            onClose={close}
            onDeleted={action.onDeleted}
          />
        )}
      </Activity>
    </ProjectActionContext.Provider>
  );
}

export function useProjectActions() {
  const actions = useContext(ProjectActionContext);
  if (!actions) throw new Error("项目操作需要 ProjectActionProvider");
  return actions;
}

export function useProjectActionActive() {
  return useContext(ProjectActionContext)?.active ?? false;
}

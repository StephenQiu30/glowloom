"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Folder } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "cn";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { useSpace } from "@/components/spaces/space-context";
import { ProjectPicker } from "@/components/projects/project-picker";
import { getProject } from "@/api/projects";
import { requireProject } from "@/components/projects/project";
import { readErrorMessage } from "@/lib/read-error";
import { ConversationWorkspace } from "./conversation-workspace";
import { useConversationContext } from "./conversation-context";
export function ConversationPage({
  projectID,
  sessionID,
}: {
  projectID?: string;
  sessionID?: string;
}) {
  const memory = useConversationContext(),
    session = useOptionalSession(),
    space = useSpace(),
    login = useLoginDialog(),
    router = useRouter();
  const [open, setOpen] = useState(false),
    [failure, setFailure] = useState<{ owner: string; text: string } | null>(
      null,
    );
  const owner = `${session?.actor.id ?? "guest"}/${space.selected?.id ?? "none"}`;
  const project = memory?.project;
  const selectProject = memory?.selectProject;
  useEffect(() => {
    if (!projectID || !session || !space.selected || project?.id === projectID)
      return;
    const controller = new AbortController();
    void getProject({ pid: projectID }, { signal: controller.signal })
      .then(requireProject)
      .then((p) => {
        if (!controller.signal.aborted) {
          selectProject?.(p);
          setFailure(null);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setFailure({ owner, text: readErrorMessage(e) });
      });
    return () => controller.abort();
  }, [projectID, owner, session, space.selected, project?.id, selectProject]);
  const selectedID = projectID ?? project?.id ?? "",
    writable = !!session && !!space.selected?.writable;
  const selectedProject = project?.id === selectedID ? project : null;
  return (
    <div
      className={cn(
        "flex h-[calc(100dvh-var(--layout-header))] min-h-[22rem] min-w-0 flex-col px-3 pb-5 md:px-6",
        sessionID ? "pt-3" : "pt-2 md:pt-11 md:pb-[21px]",
      )}
    >
      {!sessionID && (
        <PageToolbar inHeader slot="actions">
          <Button asChild variant="outline" aria-label="资产库">
            <Link href="/assets?view=reference">
              <Folder aria-hidden="true" data-icon="inline-start" />
              <span className="max-md:sr-only">资产库</span>
            </Link>
          </Button>
        </PageToolbar>
      )}
      {sessionID && (
        <header className="mx-auto mb-2 flex h-9 w-full max-w-[960px] shrink-0 items-center justify-between gap-3">
          <h1 className="truncate text-sm font-medium">对话</h1>
          {selectedProject && (
            <span className="truncate text-sm text-muted-foreground">
              {selectedProject.name}
            </span>
          )}
        </header>
      )}
      {session && !sessionID && (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent>
            <DialogTitle>选择所属项目</DialogTitle>
            <DialogDescription>
              对话历史、素材和生成权限归属所选项目。
            </DialogDescription>
            <ProjectPicker
              value={selectedProject}
              onChange={(next) => {
                const draft = memory?.readDraft("/new");
                if (
                  !selectedID &&
                  draft &&
                  !memory?.readDraft(`${next.id}/new`)
                )
                  memory?.saveDraft(`${next.id}/new`, draft);
                memory?.selectProject(next);
                setOpen(false);
                router.push(`/conversations?projectId=${next.id}`);
              }}
            />
          </DialogContent>
        </Dialog>
      )}
      {failure?.owner === owner ? (
        <p role="alert" className="mb-3 text-sm">
          {failure.text}
        </p>
      ) : null}
      <ConversationWorkspace
        key={`${owner}/${selectedID}/${sessionID ?? "new"}`}
        ownerKey={owner}
        projectID={selectedID}
        sessionID={sessionID}
        enabled={writable && !!selectedID && !(failure?.owner === owner)}
        projectSetting={
          !sessionID && session ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label="选择所属项目"
              onClick={() => setOpen(true)}
            >
              <Folder data-icon="inline-start" />
              <span className="truncate">
                {selectedProject?.name ?? "选择项目"}
              </span>
            </Button>
          ) : null
        }
        onChooseProject={() => {
          if (!session) login();
          else setOpen(true);
        }}
        onAccepted={(id) => {
          if (!sessionID)
            router.replace(`/conversations/${id}?projectId=${selectedID}`);
        }}
      />
    </div>
  );
}

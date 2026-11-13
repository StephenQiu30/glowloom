"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Folder } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ToolbarGroup } from "@/components/ui/toolbar";
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
import { CreationTypeSelect } from "@/components/generation/creation-type-select";
import {
  creationTypes,
  type CreationType,
} from "@/components/generation/creation-types";
import type { PromptDraft } from "@/components/generation/prompt-document";
import { ConversationDirectComposer } from "./conversation-direct-composer";
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
  const [selection, setSelection] = useState<{
    key: string;
    type: CreationType;
  }>({
    key: "",
    type: "agent",
  });
  const [agentLocked, setAgentLocked] = useState(false);
  const [directLocked, setDirectLocked] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [switchError, setSwitchError] = useState("");
  const switchFlight = useRef(false);
  const guard = useRef<(() => Promise<boolean>) | null>(null);
  const [drafts, setDrafts] = useState(() => new Map<string, PromptDraft>());
  const [draftIDs, setDraftIDs] = useState(() => new Map<string, string>());
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
  const scopeKey = `${owner}/${sessionID ?? "new"}`;
  const type = selection.key === scopeKey ? selection.type : "agent";
  const modeKey = `${scopeKey}/${selectedID}/${type}`;
  async function chooseProject() {
    if (!session) {
      login();
      return;
    }
    if (sessionID || agentLocked || directLocked || switchFlight.current)
      return;
    switchFlight.current = true;
    setSwitching(true);
    try {
      if (guard.current && !(await guard.current())) {
        setSwitchError(
          "当前草稿尚未确认保存，请处理保存或未决生成后再切换项目。",
        );
        return;
      }
      setOpen(true);
    } catch {
      setSwitchError("草稿保存尚未确认，当前输入已保留，请重试。");
    } finally {
      switchFlight.current = false;
      setSwitching(false);
    }
  }
  async function changeType(next: CreationType) {
    if (next === type || agentLocked || directLocked || switchFlight.current)
      return;
    switchFlight.current = true;
    setSwitching(true);
    setSwitchError("");
    try {
      if (guard.current && !(await guard.current())) {
        setSwitchError("当前草稿尚未确认保存，请处理保存或未决生成后再切换。");
        return;
      }
      setSelection({ key: scopeKey, type: next });
    } catch {
      setSwitchError("草稿保存尚未确认，当前输入已保留，请重试。");
    } finally {
      switchFlight.current = false;
      setSwitching(false);
    }
  }
  const typeControl = (
    <CreationTypeSelect
      value={type}
      disabled={agentLocked || directLocked || switching}
      onChange={(next) => void changeType(next)}
    />
  );
  const projectSetting =
    !sessionID && session ? (
      <Button
        type="button"
        variant="ghost"
        size="toolbar"
        aria-label="选择所属项目"
        disabled={switching || directLocked}
        onClick={chooseProject}
      >
        <Folder data-icon="inline-start" />
        <span className="truncate">{selectedProject?.name ?? "选择项目"}</span>
      </Button>
    ) : null;
  return (
    <div
      className={cn(
        "flex h-[calc(100dvh-var(--layout-header))] min-h-[22rem] min-w-0 flex-col px-3 pb-5 md:px-6",
        sessionID ? "pt-3" : "pt-2 md:pt-11 md:pb-[21px]",
      )}
    >
      {!sessionID && (
        <PageToolbar inHeader slot="actions">
          <ToolbarGroup aria-label="对话工具">
            <Button
              asChild
              variant="ghost"
              size="toolbar-adaptive"
              aria-label="资产库"
            >
              <Link href="/assets?view=reference">
                <Folder aria-hidden="true" data-icon="inline-start" />
                <span className="max-md:sr-only">资产库</span>
              </Link>
            </Button>
          </ToolbarGroup>
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
                if (!selectedID)
                  setDrafts((current) => {
                    const updated = new Map(current);
                    for (const kind of creationTypes) {
                      const source = `${scopeKey}//${kind.id}`;
                      const target = `${scopeKey}/${next.id}/${kind.id}`;
                      const value = current.get(source);
                      if (value && !current.has(target))
                        updated.set(target, value);
                      updated.delete(source);
                    }
                    return updated;
                  });
                memory?.selectProject(next);
                setOpen(false);
                router.push(`/conversations?projectId=${next.id}`);
              }}
            />
          </DialogContent>
        </Dialog>
      )}
      {failure?.owner === owner ? (
        <FeedbackToast message={failure.text} />
      ) : null}
      {switchError && <FeedbackToast message={switchError} />}
      <ConversationWorkspace
        key={`${owner}/${selectedID}/${sessionID ?? "new"}`}
        ownerKey={owner}
        projectID={selectedID}
        sessionID={sessionID}
        enabled={writable && !!selectedID && !(failure?.owner === owner)}
        projectSetting={projectSetting}
        headerActions={typeControl}
        onLock={setAgentLocked}
        alternateComposer={
          type === "agent" ? undefined : (
            <ConversationDirectComposer
              key={`${modeKey}/${writable && selectedProject ? "project" : "prepare"}`}
              type={type}
              contextKey={modeKey}
              project={writable ? selectedProject : null}
              draft={drafts.get(modeKey)}
              draftID={draftIDs.get(modeKey)}
              onChange={(draft) => {
                setDrafts((current) => new Map(current).set(modeKey, draft));
              }}
              onDraftReady={(id) => {
                setDraftIDs((current) =>
                  current.get(modeKey) === id
                    ? current
                    : new Map(current).set(modeKey, id),
                );
              }}
              onLock={setDirectLocked}
              onCloseGuard={(next) => {
                guard.current = next;
              }}
              onChooseProject={chooseProject}
              typeControl={typeControl}
              projectSetting={projectSetting}
            />
          )
        }
        onChooseProject={chooseProject}
        onAccepted={(id) => {
          if (!sessionID)
            router.replace(`/conversations/${id}?projectId=${selectedID}`);
        }}
      />
    </div>
  );
}

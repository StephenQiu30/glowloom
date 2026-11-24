"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Folder, History } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ProjectPicker } from "@/components/projects/project-picker";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ToolbarGroup } from "@/components/ui/toolbar";
import { cn } from "cn";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { useSpace } from "@/components/spaces/space-context";
import { readErrorMessage } from "@/lib/read-error";
import { CreationTypeSelect } from "@/components/generation/creation-type-select";
import { type CreationType } from "@/components/generation/creation-types";
import type { PromptDraft } from "@/components/generation/prompt-document";
import { ConversationDirectComposer } from "./conversation-direct-composer";
import { useConversationScope } from "./use-conversation-scope";
import { ConversationWorkspace } from "./conversation-workspace";
export function ConversationPage({
  projectID,
  sessionID,
}: {
  projectID?: string;
  sessionID?: string;
}) {
  const session = useOptionalSession(),
    space = useSpace(),
    login = useLoginDialog(),
    router = useRouter();
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
  const scope = useConversationScope(projectID);
  const selectedProject = scope.data;
  const selectedID = selectedProject?.id ?? "";
  const writable = !!session && !!space.selected?.writable;
  const [historyOpen, setHistoryOpen] = useState(false);
  const scopeKey = `${owner}/${sessionID ?? "new"}`;
  const type = selection.key === scopeKey ? selection.type : "agent";
  const modeKey = `${scopeKey}/${projectID ?? "independent"}/${type}`;
  function prepare() {
    if (!session) login();
    else if (scope.error) void scope.mutate();
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
  return (
    <div
      className={cn(
        "flex h-full min-h-[22rem] min-w-0 flex-col px-3 pb-5 md:px-6",
        sessionID ? "pt-3" : "pt-2 md:pt-11 md:pb-[21px]",
      )}
    >
      {sessionID && <h1 className="sr-only">对话</h1>}
      <PageToolbar inHeader slot="actions">
        <ToolbarGroup aria-label="对话工具">
          {session && !sessionID && (
            <Dialog open={historyOpen} onOpenChange={setHistoryOpen}>
              <DialogTrigger asChild>
                <Button
                  variant="ghost"
                  size="toolbar-icon"
                  aria-label="查看项目对话"
                  title="查看项目对话"
                >
                  <History />
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogTitle>项目对话</DialogTitle>
                <DialogDescription>选择项目查看已有对话。</DialogDescription>
                <ProjectPicker
                  value={projectID ? (selectedProject ?? null) : null}
                  onChange={(next) => {
                    setHistoryOpen(false);
                    router.push(`/conversations?projectId=${next.id}`);
                  }}
                />
              </DialogContent>
            </Dialog>
          )}
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
      {scope.error && (
        <div
          role="alert"
          className="flex items-center justify-center gap-2 text-sm"
        >
          {readErrorMessage(scope.error)}
          <Button variant="ghost" size="sm" onClick={() => void scope.mutate()}>
            重试
          </Button>
        </div>
      )}
      {switchError && <FeedbackToast message={switchError} />}
      <ConversationWorkspace
        key={`${owner}/${selectedID}/${sessionID ?? "new"}`}
        ownerKey={owner}
        projectID={selectedID}
        sessionID={sessionID}
        enabled={!!session && !!selectedID}
        writable={writable || !session}
        independent={!projectID}
        headerActions={typeControl}
        onLock={setAgentLocked}
        alternateComposer={
          type === "agent" ? undefined : (
            <ConversationDirectComposer
              key={`${modeKey}/${writable && selectedProject ? "project" : "prepare"}`}
              type={type}
              contextKey={modeKey}
              project={writable ? (selectedProject ?? null) : null}
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
              onChooseProject={prepare}
              typeControl={typeControl}
            />
          )
        }
        onChooseProject={prepare}
        onAccepted={(id) => {
          if (!sessionID)
            router.replace(
              projectID
                ? `/conversations/${id}?projectId=${projectID}`
                : `/conversations/${id}`,
            );
        }}
      />
    </div>
  );
}

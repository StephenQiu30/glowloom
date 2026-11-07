"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { ensureIndependentCreation } from "@/api/projects";
import {
  requireProject,
  isProjectID,
  type Project,
} from "@/components/projects/project";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { ConversationProvider } from "@/components/conversations/conversation-context";
import {
  createPersonalReferenceCopy,
  executePersonalReferenceCopy,
  type PersonalReferenceCopy,
} from "@/components/media/copy-personal-references";
import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { mediaBinding } from "@/components/generation/reference-controls";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ContentState } from "@/components/layout/content-state";
import { RequestError } from "@/lib/request";
import { styleError, type StyleDetail } from "./style-api";
import { parseStyleAnalysisReply } from "./style-analysis-result";
import type { StyleOwner } from "./style-library-popover";
const Workspace = dynamic(() =>
  import("@/components/conversations/conversation-workspace").then(
    (module) => module.ConversationWorkspace,
  ),
);
export type StyleAnalysisDialogProps = {
  owner: StyleOwner;
  references: StyleDetail["reference_items"];
  onUse: (text: string) => void;
  onClose: () => void;
};
export function StyleAnalysisDialog(props: StyleAnalysisDialogProps) {
  return (
    <Analysis
      key={`${props.owner.actorId}/${props.owner.scopeId}/${props.references.map((item) => `${item.id}:${item.revision}`).join(",")}`}
      {...props}
    />
  );
}
export function createStyleAnalysisDraft(
  projectID: string,
  assets: API.AssetSummary[],
): PromptDraft {
  if (
    !isProjectID(projectID) ||
    !assets.length ||
    assets.length > 4 ||
    new Set(assets.map((asset) => asset.id)).size !== assets.length ||
    assets.some(
      (asset) =>
        asset.project_id !== projectID ||
        !isProjectID(asset.id) ||
        asset.kind !== "image",
    )
  )
    throw new Error("分析参考图的项目或身份不匹配。");
  const draft = emptyPromptDraft();
  draft.references = assets.map((asset, order) => ({
    ...mediaBinding(asset, "reference_image"),
    order,
  }));
  for (const [index, reference] of draft.references.entries()) {
    draft.labelState.allocations[reference.id] = {
      labelKind: "image",
      ordinal: index + 1,
    };
  }
  draft.labelState.highWater.image = draft.references.length;
  draft.document.content = [
    {
      type: "paragraph",
      content: [
        {
          type: "text",
          text: "请分析这些参考图共有的视觉风格，提炼画面媒介、色彩、光线、构图与质感。严格用以下纯文本格式回复：第一行“风格名称：”后填写不超过 20 字的名称；空一行后以“风格描述：”开头写不超过 2000 字的完整描述。不要添加前言、Markdown 或其他内容，不要生成图片或视频，不要执行其他生成任务。",
        },
      ],
    },
    {
      type: "paragraph",
      content: draft.references.map((reference) => ({
        type: "reference",
        attrs: { referenceId: reference.id },
      })),
    },
  ];
  validatePromptDraft(draft);
  return draft;
}
function Analysis({
  owner,
  references,
  onUse,
  onClose,
}: StyleAnalysisDialogProps) {
  const session = useOptionalSession();
  const space = useSpace();
  const ready =
    !!session &&
    session.actor.id === owner.actorId &&
    !space.pending &&
    space.selected?.id === owner.scopeId &&
    !!space.selected.writable;
  const [prepared, setPrepared] = useState<{
    project: Project;
    draft: PromptDraft;
  } | null>(null);
  const [pending, setPending] = useState(true);
  const [hasCommand, setHasCommand] = useState(false);
  const [error, setError] = useState("");
  const project = useRef<Project | null>(null);
  const copy = useRef<PersonalReferenceCopy | null>(null);
  const flight = useRef<AbortController | null>(null);
  const prepare = useCallback(
    async (controller: AbortController) => {
      flight.current?.abort();
      flight.current = controller;
      setPending(true);
      setError("");
      try {
        if (!references.length || references.length > 4)
          throw new Error("请先添加 1 至 4 张参考图。");
        if (!project.current) {
          const value = requireProject(
            await ensureIndependentCreation({ signal: controller.signal }),
          );
          controller.signal.throwIfAborted();
          project.current = value;
        }
        if (!copy.current) {
          const command = await createPersonalReferenceCopy(
            project.current.id,
            references.map((item) => ({
              id: item.id,
              revision: item.revision,
              asset_id: item.asset_id,
              asset_revision: item.asset_revision,
            })),
            owner.actorId,
            controller.signal,
          );
          controller.signal.throwIfAborted();
          copy.current = command;
          setHasCommand(true);
        }
        const assets = await executePersonalReferenceCopy(
          copy.current,
          controller.signal,
        );
        controller.signal.throwIfAborted();
        setPrepared({
          project: project.current,
          draft: createStyleAnalysisDraft(project.current.id, assets),
        });
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof RequestError
              ? styleError(cause)
              : cause instanceof Error
                ? cause.message
                : "参考图准备失败，当前图片已保留。",
          );
      } finally {
        if (!controller.signal.aborted) setPending(false);
      }
    },
    [owner.actorId, references],
  );
  useEffect(() => {
    if (!ready) return;
    const controller = new AbortController();
    const timer = setTimeout(() => void prepare(controller), 0);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [ready, prepare]);
  useEffect(() => () => flight.current?.abort(), []);
  function close() {
    flight.current?.abort();
    onClose();
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent className="flex h-[min(88dvh,840px)] flex-col sm:max-w-[960px]">
        <DialogTitle>分析风格</DialogTitle>
        <DialogDescription>
          参考图就绪后，请在 Agent
          中选择可用模型并发送。分析沿用正常对话与费用确认，回复需手动采用。
        </DialogDescription>
        {!ready ? (
          <ContentState
            title="请先准备创作空间"
            description="登录并选择有权限的创作空间后，再分析参考图。"
          />
        ) : pending ? (
          <ContentState
            state="loading"
            title="正在准备分析参考图"
            description="仅复制参考图，不会自动发送分析请求。"
          />
        ) : prepared ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <ConversationProvider>
              <Workspace
                ownerKey={`${owner.actorId}/${owner.scopeId}/style-analysis`}
                projectID={prepared.project.id}
                enabled={ready}
                initialDraft={prepared.draft}
                embedded
                onChooseProject={() =>
                  setError("请在当前分析项目中选择可用的对话模型。")
                }
                onUseReply={(text) => {
                  if (!ready || !text.trim()) return;
                  try {
                    parseStyleAnalysisReply(text);
                    onUse(text);
                  } catch (cause) {
                    setError(
                      cause instanceof Error
                        ? cause.message
                        : "分析格式无法采用，请重新返回。",
                    );
                  }
                }}
                useReplyLabel="采用为风格描述"
              />
            </ConversationProvider>
          </div>
        ) : null}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {ready && !pending && !prepared && error ? (
          <Button
            variant="outline"
            onClick={() => void prepare(new AbortController())}
          >
            {hasCommand ? "重试原复制" : "重试准备参考图"}
          </Button>
        ) : null}
        <div className="flex justify-end">
          <Button variant="outline" onClick={close}>
            关闭分析
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

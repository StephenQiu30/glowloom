"use client";

import { useRef, useState } from "react";
import dynamic from "next/dynamic";
import { Workflow } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import type { CanvasEditorState } from "@/components/canvas-editor/lib/canvas-document-adapter.ts";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { serviceGraph } from "../service-graph";
import {
  compileSkillGroup,
  createSkillGroup,
  skillGroups,
  type SkillWorkflow,
  type SkillWorkflowTemplate,
} from "./skill-workflow-graph";
import templates from "./skill-workflow-templates.json";

const Conversation = dynamic(() =>
  import("@/components/conversations/conversation-workspace").then(
    (module) => module.ConversationWorkspace,
  ),
);

export function SkillWorkflowControl({
  client,
  graph,
  canvasId,
  projectId,
  identity,
  enabled,
  saved,
}: {
  client: Pick<
    CanvasYjsLiveSession,
    | "getSnapshot"
    | "canAcceptConnectionGraph"
    | "change"
    | "waitForSavedSnapshot"
  >;
  graph: CanvasEditorState;
  canvasId: string;
  projectId: string;
  identity: string;
  enabled: boolean;
  saved: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [run, setRun] = useState<SkillWorkflow>();
  const preparation = useRef(0);
  const groups = open && !run ? skillGroups(graph) : [];
  function insertTemplate(template: SkillWorkflowTemplate) {
    try {
      const current = client.getSnapshot();
      if (
        !current.document ||
        !current.connected ||
        !current.writable ||
        current.migration ||
        current.dirty ||
        current.recovery.length > 0
      )
        throw new Error("请先完成当前画布编辑，再导入技能模板");
      const { graph: next, groupId } = createSkillGroup(
        current.document,
        template,
      );
      if (!client.canAcceptConnectionGraph(next))
        throw new Error("当前画布无法加入此模板，请检查保存状态");
      client.change(next, true);
      if (
        !client
          .getSnapshot()
          .document?.nodes.some((node) => node.id === groupId)
      )
        throw new Error("技能模板未能加入，请检查画布状态");
      setOpen(false);
    } catch (error) {
      notifyError(error, "导入技能模板失败");
    }
  }
  async function openRun(id: string) {
    if (!enabled || preparing) return;
    const attempt = ++preparation.current;
    setPreparing(true);
    try {
      const { document, graphHash } = await client.waitForSavedSnapshot(() => {
        if (client.getSnapshot().recovery.length > 0)
          throw new Error("请先处理画布中待恢复的编辑");
      });
      if (preparation.current !== attempt) return;
      setRun({
        ...compileSkillGroup(serviceGraph(document, canvasId, projectId), id),
        source: {
          canvasId,
          revision: document.revision!,
          graphHash,
        },
      });
    } catch (error) {
      if (preparation.current === attempt)
        notifyError(error, "技能流程准备失败");
    } finally {
      if (preparation.current === attempt) setPreparing(false);
    }
  }
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="max-sm:size-11"
        disabled={!enabled}
        onClick={() => setOpen(true)}
      >
        <Workflow data-icon="inline-start" />
        <span className="max-sm:sr-only">技能流程</span>
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          setOpen(value);
          if (!value) {
            preparation.current++;
            setPreparing(false);
            setRun(undefined);
          }
        }}
      >
        <DialogContent
          className={
            run
              ? "h-[min(80vh,52rem)] grid-rows-[auto_minmax(0,1fr)] sm:max-w-3xl"
              : undefined
          }
        >
          <DialogHeader>
            <DialogTitle>{run ? "运行技能流程" : "技能流程"}</DialogTitle>
            <DialogDescription>
              {run
                ? "填写原著或创作要求，选择模型与预算后提交。结果随 Agent 会话保存。"
                : enabled && !saved
                  ? "画布正在保存，确认后可加入技能模板。"
                  : "将模板加入画布后，可以调整节点内容和连线。添加技能节点时，首行写 @skill 技能名，下一行填写任务。"}
            </DialogDescription>
          </DialogHeader>
          {run ? (
            <div className="min-h-0 overflow-auto">
              <Conversation
                key={run.id}
                ownerKey={identity}
                projectID={projectId}
                canvasID={canvasId}
                enabled={enabled}
                writable={enabled}
                embedded
                independent
                workflow={run}
                onChooseProject={() => setRun(undefined)}
              />
            </div>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap gap-2">
                {templates.map((template) => (
                  <Button
                    key={template.title}
                    variant="outline"
                    disabled={!enabled || !saved || preparing}
                    onClick={() => insertTemplate(template)}
                  >
                    {template.title}
                  </Button>
                ))}
              </div>
              {groups.length > 0 && (
                <div className="space-y-2">
                  {groups.map((group) => (
                    <div
                      key={group.id}
                      className="flex items-center justify-between gap-3"
                    >
                      <span className="min-w-0 truncate">{group.title}</span>
                      <Button
                        size="sm"
                        disabled={!enabled || preparing}
                        onClick={() => void openRun(group.id)}
                      >
                        运行
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

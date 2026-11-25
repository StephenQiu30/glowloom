"use client";

import { useRef, useState } from "react";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import type { CanvasPreparedDocument } from "@/components/canvas-editor/types/canvas-document-integration.ts";
import {
  compileSkillGroup,
  createSkillGroup,
  skillGroups,
  type SkillWorkflow,
} from "./skill-workflow-graph";
import templates from "./skill-workflow-templates.json";

const Conversation = dynamic(() =>
  import("@/components/conversations/conversation-workspace").then(
    (module) => module.ConversationWorkspace,
  ),
);

export function SkillWorkflowControl({
  graph,
  canvasId,
  projectId,
  identity,
  enabled,
  onEdit,
  prepare,
}: {
  graph: CanvasContract.CanvasDocumentGraphDTO;
  canvasId: string;
  projectId: string;
  identity: string;
  enabled: boolean;
  onEdit: (graph: CanvasContract.CanvasDocumentGraphDTO) => void;
  prepare: () => Promise<CanvasPreparedDocument>;
}) {
  const [open, setOpen] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [run, setRun] = useState<SkillWorkflow>();
  const preparation = useRef(0);
  const groups = open && !run ? skillGroups(graph) : [];
  async function openRun(id: string) {
    if (!enabled || preparing) return;
    const attempt = ++preparation.current;
    setPreparing(true);
    try {
      const saved = await prepare();
      if (preparation.current !== attempt) return;
      setRun({
        ...compileSkillGroup(saved.graph, id),
        source: {
          canvasId,
          revision: String(saved.revision),
          graphHash: saved.graphHash,
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
        disabled={!enabled}
        onClick={() => setOpen(true)}
      >
        技能流程
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
                    disabled={!enabled || preparing}
                    onClick={() => {
                      try {
                        onEdit(createSkillGroup(graph, template));
                        setOpen(false);
                      } catch (error) {
                        notifyError(error, "导入技能模板失败");
                      }
                    }}
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

"use client";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import type { CanvasRecoveryReview } from "./autosave-session";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import { serviceGraphOperations } from "./service-graph";
function describe(node: CanvasEditorState["nodes"][number] | undefined) {
  if (!node) return "不存在";
  return [
    node.title || "未命名节点",
    node.type === "text" ? node.metadata?.content : node.metadata?.prompt,
    `位置 ${node.position.x}, ${node.position.y}；大小 ${node.width} × ${node.height}`,
    node.documentData?.parameters
      ? `参数：${JSON.stringify(node.documentData.parameters)}`
      : "",
    node.documentData?.fileReferences?.length
      ? `已关联 ${node.documentData.fileReferences.length} 个素材引用`
      : "",
  ]
    .filter(Boolean)
    .join("\n");
}
export function CanvasRecoveryDialog({
  review,
  onClose,
  onApply,
}: {
  review: CanvasRecoveryReview;
  onClose: () => void;
  onApply: () => void;
}) {
  const operations = serviceGraphOperations(review.current, review.document);
  const ids = new Set(
    operations.flatMap((op) => {
      if (typeof op.node_id === "string") return [op.node_id];
      if (
        op.node &&
        typeof op.node === "object" &&
        "id" in op.node &&
        typeof op.node.id === "string"
      )
        return [op.node.id];
      return [];
    }),
  );
  const edges = operations.filter((op) =>
    ["connect", "disconnect", "set_edge"].includes(op.type),
  ).length;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>检查未保存的修改</DialogTitle>
          <DialogDescription>
            右侧内容将作为一次新的修改提交。服务器内容若再次变化，需要重新检查；关闭此窗口会继续保留本页输入。
          </DialogDescription>
        </DialogHeader>
        {[...ids].map((id) => (
          <div
            key={id}
            className="grid grid-cols-2 gap-3 border-b pb-3 text-sm"
          >
            <div>
              <p className="mb-2 font-medium">服务器当前内容</p>
              <pre className="font-sans break-words whitespace-pre-wrap">
                {describe(review.current.nodes.find((n) => n.id === id))}
              </pre>
            </div>
            <div>
              <p className="mb-2 font-medium">重新应用后的内容</p>
              <pre className="font-sans break-words whitespace-pre-wrap">
                {describe(review.document.nodes.find((n) => n.id === id))}
              </pre>
            </div>
          </div>
        ))}
        {edges > 0 && (
          <p className="text-sm">
            同时重新应用 {edges}{" "}
            处连线修改；端点、引用和循环检查仍由服务器执行。
          </p>
        )}
        <DialogFooter>
          <Button variant="secondary" onClick={onClose}>
            继续保留
          </Button>
          <Button onClick={onApply}>重新应用本页修改</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

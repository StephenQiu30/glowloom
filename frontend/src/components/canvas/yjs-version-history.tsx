"use client";
import { useState, type ReactNode } from "react";
import type { JSONContent } from "@tiptap/core";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import type { CanvasYjsLiveSession, YjsVersion } from "./yjs-live-session";

export function YjsVersionHistory({
  client,
  disabled,
}: {
  client: CanvasYjsLiveSession;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false),
    [versions, setVersions] = useState<YjsVersion[]>([]),
    [selected, setSelected] = useState<{
      version: YjsVersion;
      document: API.DocumentDTO;
      seq: string;
    } | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function load(before = "0") {
    setBusy(true);
    setError("");
    try {
      const page = await client.versions(before);
      setVersions((old) => (before === "0" ? page : [...old, ...page]));
    } catch {
      setError("读取历史失败，请重新打开重试。");
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button
        size="sm"
        variant="secondary"
        onClick={() => {
          setOpen(true);
          setSelected(null);
          void load();
        }}
      >
        版本历史
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value);
        }}
      >
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>画布版本历史</DialogTitle>
            <DialogDescription>
              恢复会创建新的版本，保留现有历史。画布名称与已分配引用编号保留当前值。
            </DialogDescription>
          </DialogHeader>
          {error && <p role="alert">{error}</p>}
          {busy && (
            <p role="status">正在处理；如连接中断，将查证原操作结果。</p>
          )}
          <div className="flex flex-wrap gap-2">
            {versions.map((version) => (
              <Button
                key={version.id}
                variant={
                  selected?.version.id === version.id ? "default" : "secondary"
                }
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  setError("");
                  try {
                    const document = await client.version(version.id);
                    setSelected({ version, document, seq: client.commitSeq });
                  } catch {
                    setError("无法读取该版本。");
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                版本 {version.revision} ·{" "}
                {new Date(version.createdAt).toLocaleString()}
              </Button>
            ))}
          </div>
          {versions.length >= 30 && (
            <Button
              disabled={busy}
              onClick={() => void load(versions.at(-1)!.revision)}
            >
              读取更早版本
            </Button>
          )}
          {selected && (
            <section aria-label="只读版本快照" className="space-y-2">
              <p>
                版本 {selected.version.revision}：
                {selected.document.nodes?.length ?? 0} 个节点，
                {selected.document.edges?.length ?? 0} 条连线
              </p>
              <div className="max-h-80 space-y-3 overflow-auto">
                {selected.document.nodes?.map((node) => (
                  <article
                    key={node.id}
                    className="space-y-2 rounded border p-3"
                  >
                    <h3 className="font-medium">
                      {node.title || "未命名节点"}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      位置 {node.x}, {node.y} · 尺寸 {node.width ?? "自动"} ×{" "}
                      {node.height ?? "自动"}
                    </p>
                    {node.config?.text && (
                      <p className="whitespace-pre-wrap">{node.config.text}</p>
                    )}
                    {previewText(
                      (node.config?.rich_text ?? node.config?.editor?.document)
                        ?.content as JSONContent | undefined,
                    )}
                    {node.media_ref?.asset_id && (
                      <p className="text-xs break-all">
                        素材：{node.media_ref.asset_id}
                      </p>
                    )}
                  </article>
                ))}
                {selected.document.edges?.map((edge) => (
                  <p key={edge.id} className="text-xs">
                    {selected.document.nodes?.find(
                      (n) => n.id === edge.source_node_id,
                    )?.title || "节点"}{" "}
                    →{" "}
                    {selected.document.nodes?.find(
                      (n) => n.id === edge.target_node_id,
                    )?.title || "节点"}
                  </p>
                ))}
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button variant="ghost" size="sm">
                      查看完整版本数据
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <pre className="mt-2 text-xs break-all whitespace-pre-wrap">
                      {JSON.stringify(
                        {
                          nodes: selected.document.nodes,
                          connections: selected.document.edges,
                        },
                        null,
                        2,
                      )}
                    </pre>
                  </CollapsibleContent>
                </Collapsible>
              </div>
            </section>
          )}
          <DialogFooter>
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => setOpen(false)}
            >
              关闭
            </Button>
            <Button
              disabled={busy || disabled || !selected}
              onClick={async () => {
                if (!selected) return;
                setBusy(true);
                setError("");
                try {
                  await client.restoreVersion(
                    selected.version.id,
                    selected.seq,
                  );
                  setOpen(false);
                } catch {
                  setError("恢复未执行或核对已过期，请重新选择版本后重试。");
                } finally {
                  setBusy(false);
                }
              }}
            >
              恢复所选版本
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function previewText(node: JSONContent | undefined): ReactNode {
  if (!node) return null;
  let content: ReactNode =
    node.text ??
    node.content?.map((child, index) => (
      <span key={index}>{previewText(child)}</span>
    ));
  for (const mark of node.marks ?? []) {
    if (mark.type === "bold") content = <strong>{content}</strong>;
    if (mark.type === "italic") content = <em>{content}</em>;
    if (mark.type === "strike") content = <s>{content}</s>;
    if (mark.type === "underline") content = <u>{content}</u>;
  }
  if (node.type === "hardBreak") return <br />;
  if (node.type === "mention" || node.type === "reference")
    return (
      <span className="rounded bg-muted px-1">
        {node.attrs?.label ?? "素材引用"}
      </span>
    );
  if (node.type === "paragraph" || node.type === "heading")
    return <div className="whitespace-pre-wrap">{content}</div>;
  return content;
}

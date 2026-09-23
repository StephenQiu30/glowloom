"use client";

import { useState } from "react";
import Link from "next/link";
import { ArrowUpRight, Copy, Link2, Trash2, Unlink2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { CanvasNode } from "./canvas-model";

export function CanvasInspector({
  node,
  canEdit,
  pending,
  connecting,
  connections,
  nodes,
  onSave,
  onDuplicate,
  onDelete,
  onConnect,
  onDisconnect,
}: {
  node: CanvasNode;
  canEdit: boolean;
  pending: boolean;
  connecting: boolean;
  connections: GeneratedAPI.CanvasConnection[];
  nodes: CanvasNode[];
  onSave: (changes: { title: string; content: string; prompt: string }) => void;
  onDuplicate: () => void;
  onDelete: () => void;
  onConnect: () => void;
  onDisconnect: (edge: GeneratedAPI.CanvasConnection) => void;
}) {
  const [title, setTitle] = useState(node.title);
  const [content, setContent] = useState(node.saved?.content ?? "");
  const [prompt, setPrompt] = useState(node.saved?.prompt ?? "");
  const outgoing = connections.filter((edge) => edge.from_node_id === node.id);
  const dirty = title !== node.title || content !== (node.saved?.content ?? "") || prompt !== (node.saved?.prompt ?? "");

  return (
    <aside aria-label="选中节点详情" className="absolute bottom-5 left-5 right-5 z-10 max-h-[55%] overflow-y-auto rounded-2xl bg-background/95 p-5 shadow-lg backdrop-blur-sm md:bottom-5 md:right-auto md:w-72">
      <p className="text-xs font-medium uppercase tracking-widest text-muted-foreground">
        {node.kind === "project" ? "项目" : node.kind === "episode" ? "单集" : node.kind}
      </p>
      <h2 className="mt-2 truncate text-base font-semibold">{node.title}</h2>
      {!node.saved ? (
        <>
          <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{node.subtitle}</p>
          {node.href ? (
            <Link className="mt-4 inline-flex items-center gap-1 text-sm font-medium hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring" href={node.href}>
              进入工作区 <ArrowUpRight className="size-4" aria-hidden="true" />
            </Link>
          ) : null}
        </>
      ) : (
        <div className="mt-4 space-y-3">
          <label className="block space-y-1 text-xs text-muted-foreground">
            <span>标题</span>
            <Input disabled={!canEdit || pending} maxLength={120} onChange={(event) => setTitle(event.target.value)} value={title} />
          </label>
          {node.kind === "text" || node.kind === "note" ? (
            <label className="block space-y-1 text-xs text-muted-foreground">
              <span>内容</span>
              <Textarea disabled={!canEdit || pending} maxLength={16000} onChange={(event) => setContent(event.target.value)} rows={3} value={content} />
            </label>
          ) : node.kind !== "group" ? (
            <label className="block space-y-1 text-xs text-muted-foreground">
              <span>创作提示</span>
              <Textarea disabled={!canEdit || pending} maxLength={32000} onChange={(event) => setPrompt(event.target.value)} rows={3} value={prompt} />
            </label>
          ) : null}
          {node.saved.media_version_id ? (
            <p className="text-xs text-muted-foreground">媒体版本：{node.saved.media_version_id}</p>
          ) : null}
          {canEdit ? (
            <div className="flex flex-wrap gap-1.5">
              <Button disabled={!dirty || !title.trim() || pending} onClick={() => onSave({ title: title.trim(), content, prompt })} size="sm">保存</Button>
              <Button aria-label="复制节点" disabled={pending} onClick={onDuplicate} size="icon-sm" variant="ghost"><Copy aria-hidden="true" /></Button>
              <Button aria-label={connecting ? "取消连接" : "连接到其他节点"} disabled={pending} onClick={onConnect} size="icon-sm" variant="ghost"><Link2 aria-hidden="true" /></Button>
              <Button aria-label="删除节点" disabled={pending} onClick={onDelete} size="icon-sm" variant="ghost"><Trash2 aria-hidden="true" /></Button>
            </div>
          ) : null}
          {connecting ? <p className="text-xs text-muted-foreground">选择另一个媒体或文本节点，建立视觉连线。</p> : null}
          {outgoing.length ? (
            <div className="space-y-1 pt-1">
              <p className="text-xs text-muted-foreground">视觉连线</p>
              {outgoing.map((edge) => (
                <div className="flex items-center justify-between gap-2 text-xs" key={edge.id}>
                  <span className="truncate">→ {nodes.find((item) => item.id === edge.to_node_id)?.title ?? "已移除节点"}</span>
                  {canEdit ? <Button aria-label="删除视觉连线" disabled={pending} onClick={() => onDisconnect(edge)} size="icon-xs" variant="ghost"><Unlink2 aria-hidden="true" /></Button> : null}
                </div>
              ))}
            </div>
          ) : null}
        </div>
      )}
    </aside>
  );
}

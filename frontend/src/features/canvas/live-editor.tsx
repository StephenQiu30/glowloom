"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  applyNodeChanges,
  useReactFlow,
  type Connection,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useTheme } from "next-themes";
import { useStore } from "zustand";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ApiError } from "@/lib/request";
import {
  applyCommands,
  createEditor,
  edit,
  redo,
  undo,
  type CanvasCommand,
  type CanvasDocument,
  type CanvasEditor,
} from "./live-state";
import { createCanvasStore, type NoteNode } from "./live-store";

function NoteCard({ data, selected }: NodeProps<NoteNode>) {
  return (
    <div
      className={`h-full min-h-24 min-w-40 rounded-xl bg-background p-5 ${selected ? "ring-2 ring-blue-500" : ""}`}
    >
      <Handle type="target" position={Position.Left} aria-label="注释线终点" />
      <p className="mb-2 text-xs text-muted-foreground">文字备注</p>
      <p className="line-clamp-8 text-sm leading-6 break-words whitespace-pre-wrap">
        {data.text || "空备注"}
      </p>
      <Handle type="source" position={Position.Right} aria-label="注释线起点" />
    </div>
  );
}
const nodeTypes = { note: NoteCard };
type Props = {
  document: CanvasDocument;
  readOnly: boolean;
  save: (
    revision: number,
    commands: CanvasCommand[],
    key: string,
  ) => Promise<CanvasDocument>;
  reload: () => Promise<CanvasDocument>;
  onAuthFailure: (error: ApiError) => void;
  onDirtyChange?: (dirty: boolean) => void;
};
type WriteKind = "edit" | "undo" | "redo";
type Attempt = {
  commands: CanvasCommand[];
  revision: number;
  key: string;
  kind: WriteKind;
};

export function LiveEditor(props: Props) {
  return (
    <ReactFlowProvider>
      <Editor {...props} />
    </ReactFlowProvider>
  );
}

function Editor({
  document: initial,
  readOnly,
  save,
  reload,
  onAuthFailure,
  onDirtyChange,
}: Props) {
  const [store] = useState(() => createCanvasStore(initial));
  const editor = useStore(store, (state) => state.editor);
  const nodes = useStore(store, (state) => state.nodes);
  const selectedId = useStore(store, (state) => state.selectedId);
  const selectedEdge = useStore(store, (state) => state.selectedEdge);
  const text = useStore(store, (state) => state.text);
  const setText = useStore(store, (state) => state.setText);
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState<{ attempt: Attempt; error: Error }>();
  const [notice, setNotice] = useState("已读取服务端文档。");
  const [leaveHref, setLeaveHref] = useState<string>();
  const leaving = useRef(false);
  const busy = useRef(false);
  const flow = useReactFlow<NoteNode>();
  const { resolvedTheme } = useTheme();
  const locked = readOnly || saving || Boolean(failed);
  const selected = editor.document.nodes.find((node) => node.id === selectedId);
  const textDirty = Boolean(selected && text !== selected.config.text);
  const dirty = saving || Boolean(failed) || textDirty;
  useEffect(() => {
    onDirtyChange?.(dirty);
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      if (!leaving.current) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    const link = (event: MouseEvent) => {
      if (
        event.button ||
        event.ctrlKey ||
        event.metaKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const anchor =
        event.target instanceof Element
          ? event.target.closest("a[href]")
          : null;
      if (
        !(anchor instanceof HTMLAnchorElement) ||
        anchor.target === "_blank" ||
        anchor.hasAttribute("download")
      )
        return;
      const target = new URL(anchor.href);
      if (
        target.origin !== window.location.origin ||
        target.href === window.location.href ||
        anchor.getAttribute("href")?.startsWith("#")
      )
        return;
      event.preventDefault();
      event.stopPropagation();
      setLeaveHref(target.href);
    };
    window.addEventListener("beforeunload", warn);
    document.addEventListener("click", link, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      document.removeEventListener("click", link, true);
    };
  }, [dirty, onDirtyChange]);
  const edges = useMemo(
    () =>
      editor.document.edges.map((edge) => ({
        id: edge.id,
        source: edge.source_node_id,
        target: edge.target_node_id,
        type: "straight",
        label: "注释",
        selected: edge.id === selectedEdge,
      })),
    [editor.document.edges, selectedEdge],
  );

  function selectNote(id: string) {
    if (selectedId === id) return;
    if (textDirty) {
      setNotice("请先保存当前备注或放弃文字草稿，再选择其他备注。");
      return;
    }
    store.getState().selectNote(id);
  }
  function calibrate(next: CanvasEditor, syncText = false) {
    store.getState().calibrate(next, false, syncText);
    void flow.setViewport(next.document.viewport, { duration: 0 });
  }
  async function persist(attempt: Attempt) {
    if (busy.current || readOnly) return;
    busy.current = true;
    setSaving(true);
    setNotice("正在等待服务端确认…");
    try {
      applyCommands(editor.document, attempt.commands);
      const saved = await save(attempt.revision, attempt.commands, attempt.key);
      const next =
        attempt.kind === "undo"
          ? undo(editor, saved)
          : attempt.kind === "redo"
            ? redo(editor, saved)
            : edit(editor, attempt.commands, saved);
      calibrate(
        next,
        attempt.commands.some(
          (command) =>
            command.type === "UpdateNodeConfig" && command.id === selectedId,
        ),
      );
      setFailed(undefined);
      setNotice(`已保存至服务端 · 修订 ${saved.revision}`);
    } catch (error) {
      const failure =
        error instanceof Error ? error : new Error("保存未完成。");
      setFailed({ attempt, error: failure });
      setNotice("修改尚未确认保存。");
      if (failure instanceof ApiError && failure.status === 401)
        onAuthFailure(failure);
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }
  function submit(commands: CanvasCommand[], kind: WriteKind = "edit") {
    if (locked || !commands.length) return;
    void persist({
      commands,
      kind,
      key: crypto.randomUUID(),
      revision: editor.document.revision,
    });
  }
  async function readLatest() {
    if (busy.current) return;
    busy.current = true;
    setSaving(true);
    try {
      const latest = await reload();
      store.getState().calibrate(createEditor(latest), true);
      calibrate(createEditor(latest));
      setFailed(undefined);
      setNotice(`已载入最新修订 ${latest.revision}，本页撤销历史已清空。`);
    } catch (error) {
      const failure = error instanceof Error ? error : new Error("读取失败。");
      setNotice(failure.message);
      if (failure instanceof ApiError && failure.status === 401)
        onAuthFailure(failure);
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }
  function connect(connection: Connection) {
    if (!connection.source || !connection.target) return;
    submit([
      {
        type: "Connect",
        edge: {
          id: crypto.randomUUID(),
          edge_type: "annotation",
          source_node_id: connection.source,
          target_node_id: connection.target,
        },
      },
    ]);
  }

  return (
    <section aria-label="真实备注画布" className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold">{editor.document.name}</h2>
          <p
            role="status"
            aria-live="polite"
            className="mt-2 text-xs text-muted-foreground"
          >
            {notice}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={
              locked || textDirty || editor.document.nodes.length >= 2000
            }
            onClick={() => {
              const position = flow.screenToFlowPosition({
                x: window.innerWidth / 2,
                y: window.innerHeight / 2,
              });
              submit([
                {
                  type: "AddNodes",
                  nodes: [
                    {
                      id: crypto.randomUUID(),
                      node_type: "text",
                      node_action: "resource",
                      config: { text: "新备注" },
                      ...position,
                      width: 240,
                      height: 160,
                    },
                  ],
                },
              ]);
            }}
          >
            新增备注
          </Button>
          <Button
            variant="secondary"
            disabled={locked || textDirty || !editor.past.length}
            onClick={() => submit(editor.past.at(-1)!.inverse, "undo")}
          >
            撤销
          </Button>
          <Button
            variant="secondary"
            disabled={locked || textDirty || !editor.future.length}
            onClick={() => submit(editor.future.at(-1)!.forward, "redo")}
          >
            重做
          </Button>
          <Button
            variant="secondary"
            disabled={locked}
            onClick={() =>
              submit([{ type: "SetViewport", viewport: flow.getViewport() }])
            }
          >
            保存当前视口
          </Button>
          <Button
            variant="ghost"
            disabled={saving || Boolean(failed)}
            onClick={() => void readLatest()}
          >
            {textDirty ? "放弃文字草稿并读取最新" : "读取最新"}
          </Button>
        </div>
      </div>
      {readOnly && (
        <p role="status" className="rounded-lg bg-muted p-4 text-sm">
          项目已归档。可以查看备注和布局，无法修改。
        </p>
      )}
      {failed && (
        <div
          role="alert"
          className="space-y-3 rounded-lg bg-destructive/10 p-4 text-sm"
        >
          <p>{failed.error.message}</p>
          {failed.error instanceof ApiError && failed.error.requestId && (
            <p className="text-xs">请求编号：{failed.error.requestId}</p>
          )}
          <p>
            失败操作仍留在本页，尚未确认保存。读取最新后请重新操作，撤销历史将清空。
          </p>
          <div className="flex flex-wrap gap-2">
            {!(
              failed.error instanceof ApiError &&
              [401, 403, 409].includes(failed.error.status)
            ) && (
              <Button
                variant="secondary"
                disabled={saving}
                onClick={() => void persist(failed.attempt)}
              >
                重试保存
              </Button>
            )}
            <Button
              variant="secondary"
              disabled={saving}
              onClick={() => void readLatest()}
            >
              放弃本页失败修改并读取最新
            </Button>
          </div>
        </div>
      )}
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_280px]">
        <div
          className="h-[600px] overflow-hidden rounded-xl bg-muted/40"
          role="region"
          aria-label="画布编辑区"
        >
          <ReactFlow<NoteNode>
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={(changes) =>
              store
                .getState()
                .setNodes(applyNodeChanges(changes, store.getState().nodes))
            }
            defaultViewport={initial.viewport}
            minZoom={0.1}
            maxZoom={4}
            colorMode={resolvedTheme === "dark" ? "dark" : "light"}
            nodesDraggable={!locked && !textDirty}
            nodesConnectable={!locked && !textDirty}
            elementsSelectable
            deleteKeyCode={null}
            onlyRenderVisibleElements
            onNodeClick={(_, node) => selectNote(node.id)}
            onEdgeClick={(_, edge) => {
              if (!textDirty) store.getState().selectEdge(edge.id);
            }}
            onNodeDragStop={(_, node, movedNodes) => {
              const moved = (movedNodes.length ? movedNodes : [node]).filter(
                (item) => {
                  const original = editor.document.nodes.find(
                    (candidate) => candidate.id === item.id,
                  );
                  return (
                    original &&
                    (original.x !== item.position.x ||
                      original.y !== item.position.y)
                  );
                },
              );
              if (moved.length)
                submit([
                  {
                    type: "MoveNodes",
                    moves: moved.map((item) => ({
                      id: item.id,
                      ...item.position,
                    })),
                  },
                ]);
            }}
            onConnect={connect}
            onMoveEnd={(event, viewport) => {
              const previous = editor.document.viewport;
              if (
                event &&
                !locked &&
                (previous.x !== viewport.x ||
                  previous.y !== viewport.y ||
                  previous.zoom !== viewport.zoom)
              )
                submit([{ type: "SetViewport", viewport }]);
            }}
            ariaLabelConfig={{
              "controls.zoomIn.ariaLabel": "放大画布",
              "controls.zoomOut.ariaLabel": "缩小画布",
              "controls.fitView.ariaLabel": "显示全部备注",
            }}
          >
            <Background gap={24} />
            <MiniMap pannable zoomable />
            <Controls showInteractive={false} />
          </ReactFlow>
        </div>
        <aside
          className="space-y-6 rounded-xl bg-muted/30 p-5"
          aria-label="备注属性"
        >
          {selected ? (
            <>
              <Field>
                <FieldLabel htmlFor="canvas-note">备注内容</FieldLabel>
                <Textarea
                  id="canvas-note"
                  value={text}
                  onChange={(event) => setText(event.target.value)}
                  disabled={locked}
                  className="min-h-48"
                />
              </Field>
              <p className="text-xs text-muted-foreground">
                {Array.from(text).length} / 10,000 字符
              </p>
              {text !== selected.config.text && (
                <p role="status" className="text-xs text-muted-foreground">
                  备注文字仍是本页草稿，点击“保存备注”后才写入服务端。
                </p>
              )}
              {textDirty && (
                <Button
                  variant="ghost"
                  disabled={saving}
                  onClick={() => setText(selected.config.text)}
                >
                  放弃文字草稿
                </Button>
              )}
              <Button
                disabled={
                  locked ||
                  text === selected.config.text ||
                  Array.from(text).length > 10000
                }
                onClick={() =>
                  submit([
                    {
                      type: "UpdateNodeConfig",
                      id: selected.id,
                      config: { text },
                    },
                  ])
                }
              >
                保存备注
              </Button>
              <Button
                variant="ghost"
                disabled={locked}
                onClick={() =>
                  submit([{ type: "DeleteNodes", ids: [selected.id] }])
                }
              >
                删除备注及相连注释线
              </Button>
            </>
          ) : selectedEdge ? (
            <>
              <h3 className="font-medium">注释连接</h3>
              <p className="text-sm leading-6 text-muted-foreground">
                这条线只表达备注之间的关系。
              </p>
              <Button
                variant="secondary"
                disabled={locked}
                onClick={() =>
                  submit([{ type: "Disconnect", id: selectedEdge }])
                }
              >
                删除注释线
              </Button>
            </>
          ) : (
            <p className="text-sm leading-7 text-muted-foreground">
              选择备注后编辑文字。拖动备注保存布局；从右侧连接点拖到另一个备注左侧，创建注释线。
            </p>
          )}
          <p className="text-xs leading-6 text-muted-foreground">
            本轮只保存文字备注、布局、视口与注释线。参考、提升、运行生成及业务数据同步将在后续功能验收。
          </p>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" disabled>
              参考连线
            </Button>
            <Button size="sm" disabled>
              提升结果
            </Button>
            <Button size="sm" disabled>
              运行生成
            </Button>
          </div>
          <p className="font-mono text-xs text-muted-foreground">
            {editor.document.nodes.length} 备注 / {editor.document.edges.length}{" "}
            注释线
          </p>
        </aside>
      </div>
      <Dialog
        open={Boolean(leaveHref)}
        onOpenChange={(open) => {
          if (!open) setLeaveHref(undefined);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>离开当前画布？</DialogTitle>
            <DialogDescription>
              本页仍有未保存草稿、失败操作或等待确认的请求。离开后这些内容不会自动保存。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="secondary" onClick={() => setLeaveHref(undefined)}>
              继续编辑
            </Button>
            <Button
              variant="destructive"
              disabled={saving}
              onClick={() => {
                if (leaveHref) {
                  leaving.current = true;
                  window.location.assign(leaveHref);
                }
              }}
            >
              放弃并离开
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

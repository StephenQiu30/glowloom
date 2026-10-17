"use client";

import { FeedbackToast } from "@/components/feedback/feedback-toast";

import {
  useCallback,
  useEffectEvent,
  useEffect,
  useRef,
  useState,
  type PointerEvent,
} from "react";
import Link from "next/link";
import dynamic from "next/dynamic";
import { ArrowLeft, Check, CircleAlert, Plus, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Brand } from "@/components/brand/brand";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyContent,
  EmptyTitle,
} from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { UploadDialog } from "@/components/media/upload-dialog";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { subjectText, type Subject } from "@/components/subjects/contracts";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  GenerationDialog,
  type GenerationSeed,
} from "@/components/generation/generation-dialog";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { getSession } from "@/api/identity";
import {
  branchJournal,
  canonical,
  type LocalBranch,
} from "@/components/generation/draft-save";
import { RequestError } from "@/lib/request";
import { applyCanvasCommands, getCanvas } from "@/api/canvases";
import type { Project } from "@/components/projects/project";
import {
  commandBatch,
  richCanvasDraft,
  createDraftNode,
  createMediaNode,
  duplicateNode,
  isDraft,
  isEditableNode,
  requireCanvas,
  requireSaveResult,
  sameEditable,
  type Canvas,
  type Draft,
  type Node,
} from "./editor-state";

import { InfiniteCanvas } from "./infinite-canvas";
import { CanvasNode } from "./canvas-node";
import { CanvasToolbar } from "./canvas-toolbar";
import { CanvasComposer } from "./canvas-composer";
import { CanvasMiniMap } from "./canvas-mini-map";
import {
  CANVAS_NODE_DRAG_EVENT,
  type SelectionArea,
  type ViewportTransform,
} from "./canvas-viewport";
import { CanvasNodePanel } from "./canvas-node-panel";
import "./canvas.css";

type Pending = {
  key: string;
  base: Canvas;
  nodes: Node[];
  body: API.CommandsInput;
};
const limit = (value: number) => Math.max(-1e6, Math.min(1e6, value));
const CanvasMaterialDialog = dynamic(() =>
  import("./canvas-material-dialog").then(
    (module) => module.CanvasMaterialDialog,
  ),
);

export function CanvasEditor({
  project,
  initial,
}: {
  project: Project;
  initial: Canvas;
}) {
  const [generation, setGeneration] = useState<GenerationSeed | null>(null);
  const [materials, setMaterials] = useState<{
    tab: "assets" | "subjects";
    kind?: string;
  } | null>(null);
  const [upload, setUpload] = useState(false);
  const [preview, setPreview] = useState<Node | null>(null);
  const [base, setBaseState] = useState(initial);
  const savedBase = useRef(initial);
  const setBase = (value: Canvas) => {
    savedBase.current = value;
    setBaseState(value);
  };
  const privateInvalid = useRef(false);
  const [identityInvalid, setIdentityInvalid] = useState(false);
  const [nodes, setNodes] = useState(initial.nodes);
  const local = useRef(initial.nodes);
  const [selected, setSelected] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [message, setMessage] = useState("");
  const [conflict, setConflict] = useState<{ server: Canvas | null } | null>(
    null,
  );
  const [leave, setLeave] = useState(false);
  const leaving = useRef(false);
  const pending = useRef<Pending | null>(null);
  const busy = useRef(false);
  const alive = useRef(false);
  const request = useRef<AbortController | null>(null);
  const board = useRef<HTMLDivElement>(null);
  const [tool, setTool] = useState<"select" | "pan">("select");
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [details, setDetails] = useState(false);
  const [miniMap, setMiniMap] = useState(false);
  const [size, setSize] = useState({ width: 1024, height: 768 });
  const [camera, setCamera] = useState({ x: 0, y: 0, zoom: 1 });
  const view = useRef(camera);
  const drag = useRef<{
    id: string;
    clientX: number;
    clientY: number;
    x: number;
    y: number;
    nextX: number;
    nextY: number;
    card: HTMLElement;
    button: HTMLButtonElement;
    pointerId: number;
    members: {
      id: string;
      x: number;
      y: number;
      element: HTMLElement | null;
    }[];
  } | null>(null);
  const baseline = new Map(base.nodes.map((n) => [n.id, n]));
  const dirty =
    nodes.length !== base.nodes.length ||
    nodes.some(
      (n) => !baseline.has(n.id) || !sameEditable(n, baseline.get(n.id)!),
    );
  const current = nodes.find((n) => n.id === selected);
  const [replicaId] = useState(() => crypto.randomUUID());
  const [recoveryContext, setRecoveryContext] = useState("");
  const [recoveryBranches, setRecoveryBranches] = useState<LocalBranch[]>([]);
  const recordRevision = useRef(0),
    journal = useRef(Promise.resolve());
  useEffect(() => {
    let active = true;
    getSession()
      .then(async (identity) => {
        if (!identity.actor?.id) throw new Error("session_invalid");
        const context = `${identity.actor.id}/${project.id}/canvas/${initial.id}`;
        const branches = await branchJournal.list(context);
        if (active) {
          setRecoveryContext(context);
          setRecoveryBranches(
            branches.filter(
              (b) =>
                b.replicaId !== replicaId && b.ackedSeq !== b.snapshot.localSeq,
            ),
          );
        }
      })
      .catch(() => {
        if (active)
          setMessage("本地恢复待核验；请保持当前页面并确认登录状态。");
      });
    return () => {
      active = false;
    };
  }, [project.id, initial.id, replicaId]);
  useEffect(() => {
    const suspend = () => {
      privateInvalid.current = true;
      request.current?.abort();
      setIdentityInvalid(true);
      setGeneration(null);
    };
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("glowloom-private-cache")
        : null;
    if (channel) channel.onmessage = suspend;
    window.addEventListener("glowloom:private-cache-cleared", suspend);
    const verify = () => {
      if (document.visibilityState === "visible" && recoveryContext) {
        void getSession()
          .then((identity) => {
            if (
              !identity.actor?.id ||
              !recoveryContext.startsWith(identity.actor.id + "/")
            )
              suspend();
          })
          .catch(suspend);
      }
    };
    document.addEventListener("visibilitychange", verify);
    return () => {
      channel?.close();
      window.removeEventListener("glowloom:private-cache-cleared", suspend);
      document.removeEventListener("visibilitychange", verify);
    };
  }, [recoveryContext]);
  const persistCanvas = useCallback(async () => {
    if (!recoveryContext || privateInvalid.current) return;
    const revision = ++recordRevision.current;
    const record: LocalBranch = {
      contextKey: recoveryContext,
      replicaId,
      localRecordRevision: revision,
      snapshot: {
        contextKey: recoveryContext,
        replicaId,
        localSeq: revision,
        draft: emptyPromptDraft(),
      },
      serverRevision: savedBase.current.revision,
      ackedSeq: dirty || pending.current ? -1 : revision,
      ownerState: {
        base: savedBase.current,
        nodes: structuredClone(local.current),
        pending: structuredClone(pending.current),
      },
    };
    journal.current = journal.current
      .catch(() => {})
      .then(() =>
        privateInvalid.current ? undefined : branchJournal.write(record),
      );
    try {
      await journal.current;
    } catch {
      if (alive.current)
        setMessage("本地恢复不可用；请保存到服务端后再关闭页面。");
    }
  }, [recoveryContext, replicaId, dirty]);
  useEffect(() => {
    void persistCanvas();
  }, [nodes, base, recoveryContext, saving, unconfirmed, persistCanvas]);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);
  useEffect(() => {
    if (!dirty && !saving && !unconfirmed) return;
    const warn = (e: BeforeUnloadEvent) => {
      if (leaving.current) return;
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty, saving, unconfirmed]);

  useEffect(() => {
    const el = board.current;
    if (!el) return;
    const measure = () =>
      setSize({
        width: el.clientWidth || 1024,
        height: el.clientHeight || 768,
      });
    measure();
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(measure);
    observer?.observe(el);
    window.addEventListener("resize", measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey)
        return;
      if (
        event.target instanceof Element &&
        event.target.closest(
          "input,textarea,select,button,[contenteditable=true],[role=dialog],[role=alertdialog],[role=menu],[role=listbox]",
        )
      )
        return;
      if (event.key.toLowerCase() === "v" || event.key.toLowerCase() === "h") {
        event.preventDefault();
        setTool(event.key.toLowerCase() === "h" ? "pan" : "select");
      }
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, []);
  const viewportChange = useCallback((next: ViewportTransform) => {
    view.current = { x: next.x, y: next.y, zoom: next.k };
    setCamera(view.current);
  }, []);
  const viewportPreview = useCallback((next: ViewportTransform) => {
    view.current = { x: next.x, y: next.y, zoom: next.k };
  }, []);
  function selectNode(id?: string) {
    if (id !== selected) setDetails(false);
    setSelected(id);
    setSelectedIDs(id ? [id] : []);
  }
  function selectArea(area: SelectionArea) {
    const ids = local.current
      .filter(
        (node) =>
          node.x <= area.x + area.width &&
          node.x + (node.width ?? 288) >= area.x &&
          node.y <= area.y + area.height &&
          node.y + (node.height ?? 200) >= area.y,
      )
      .map((node) => node.id);
    setSelectedIDs(ids);
    setSelected(ids.at(-1));
  }
  function edit(next: Node[]) {
    local.current = next;
    setNodes(next);
  }
  function update(id: string, patch: Partial<Node>) {
    edit(local.current.map((n) => (n.id === id ? { ...n, ...patch } : n)));
  }
  function draft(patch: Partial<Draft>) {
    const n = local.current.find((n) => n.id === selected);
    if (n && isDraft(n))
      update(n.id, { config: { draft: { ...n.config.draft, ...patch } } });
  }
  function add(kind: "image_generation" | "video_generation") {
    if (local.current.length >= 2000) {
      setMessage("画布已达到 2000 个节点上限。");
      return;
    }
    const n = createDraftNode(kind, project.aspect_ratio, local.current.length);
    const bounds = board.current?.getBoundingClientRect();
    if (bounds?.width && bounds.height) {
      n.x = limit(
        Math.round(
          (bounds.width / 2 - view.current.x) / view.current.zoom - 144,
        ),
      );
      n.y = limit(
        Math.round(
          (Math.max(96, (bounds.height - 450) / 2) - view.current.y) /
            view.current.zoom,
        ),
      );
    }
    edit([...local.current, n]);
    selectNode(n.id);
  }
  function addMaterials(assets: API.AssetSummary[]) {
    if (local.current.length + assets.length > 2000) {
      setMessage("画布已达到 2000 个节点上限，请减少所选素材。");
      return;
    }
    try {
      const added = assets.map((a, i) =>
        createMediaNode(a, project.id, local.current.length + i),
      );
      const bounds = board.current?.getBoundingClientRect();
      if (bounds?.width && bounds.height) {
        const x = (bounds.width / 2 - view.current.x) / view.current.zoom - 144;
        const y =
          (Math.max(96, (bounds.height - 450) / 2) - view.current.y) /
          view.current.zoom;
        added.forEach((n, i) => {
          n.x = limit(Math.round(x + (i % 3) * 340));
          n.y = limit(Math.round(y + Math.floor(i / 3) * 260));
        });
      }
      edit([...local.current, ...added]);
      selectNode(added.at(-1)?.id);
      setMaterials(null);
      setMessage(`已添加 ${added.length} 个素材，请保存画布。`);
    } catch {
      setMessage("素材信息不完整或不属于当前项目，请重新读取后添加。");
    }
  }
  function copyNode(node: Node) {
    if (local.current.length >= 2000) {
      setMessage("画布已达到 2000 个节点上限。");
      return;
    }
    const copy = duplicateNode(node);
    edit([...local.current, copy]);
    selectNode(copy.id);
  }
  function removeLocal(node: Node) {
    if (
      busy.current ||
      unconfirmed ||
      conflict ||
      baseline.has(node.id) ||
      !isEditableNode(node)
    )
      return;
    edit(local.current.filter((n) => n.id !== node.id));
    selectNode(undefined);
  }
  function useSubject(subject: Subject) {
    setMaterials(null);
    setGeneration({
      project,
      prompt: `${subjectText(subject).name}。${subjectText(subject).description ?? ""}`,
      capability: "image",
      subject,
    });
  }
  function changeCamera(next: typeof camera) {
    view.current = next;
    setCamera(next);
  }
  function fit() {
    if (!board.current || !nodes.length) {
      changeCamera({ x: 0, y: 0, zoom: 1 });
      return;
    }
    const minX = Math.min(...nodes.map((n) => n.x)),
      minY = Math.min(...nodes.map((n) => n.y));
    const maxX = Math.max(...nodes.map((n) => n.x + (n.width ?? 288))),
      maxY = Math.max(...nodes.map((n) => n.y + (n.height ?? 200)));
    const { width, height } = board.current.getBoundingClientRect();
    const zoom = Math.max(
      0.05,
      Math.min(2, (width - 96) / (maxX - minX), (height - 192) / (maxY - minY)),
    );
    changeCamera({ x: 48 - minX * zoom, y: 96 - minY * zoom, zoom });
  }
  function beginDrag(e: PointerEvent<HTMLButtonElement>, node: Node) {
    if (!isEditableNode(node) || e.button !== 0 || tool === "pan") return;
    e.stopPropagation();
    if (!selectedIDs.includes(node.id)) selectNode(node.id);
    e.currentTarget.setPointerCapture(e.pointerId);
    const card = e.currentTarget.parentElement!;
    drag.current = {
      id: node.id,
      clientX: e.clientX,
      clientY: e.clientY,
      x: node.x,
      y: node.y,
      nextX: node.x,
      nextY: node.y,
      card,
      button: e.currentTarget,
      pointerId: e.pointerId,
      members: local.current
        .filter(
          (n) =>
            isEditableNode(n) &&
            (selectedIDs.includes(node.id)
              ? selectedIDs.includes(n.id)
              : n.id === node.id),
        )
        .map((n) => ({
          id: n.id,
          x: n.x,
          y: n.y,
          element:
            board.current?.querySelector<HTMLElement>(
              `[data-node-id="${n.id}"]`,
            ) ?? null,
        })),
    };
  }
  function moveDrag(e: PointerEvent<HTMLButtonElement>) {
    const d = drag.current;
    if (!d) return;
    d.nextX = limit(
      Math.round(d.x + (e.clientX - d.clientX) / view.current.zoom),
    );
    d.nextY = limit(
      Math.round(d.y + (e.clientY - d.clientY) / view.current.zoom),
    );
    for (const member of d.members) {
      if (member.element) {
        member.element.style.left = `${limit(member.x + d.nextX - d.x)}px`;
        member.element.style.top = `${limit(member.y + d.nextY - d.y)}px`;
      }
    }
    board.current?.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT));
  }
  function endDrag(cancel = false) {
    const d = drag.current;
    if (!d) return;
    drag.current = null;
    if (d.button.hasPointerCapture(d.pointerId))
      d.button.releasePointerCapture(d.pointerId);
    if (cancel) {
      for (const member of d.members)
        if (member.element) {
          member.element.style.left = `${member.x}px`;
          member.element.style.top = `${member.y}px`;
        }
      board.current?.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT));
      return;
    }
    const moved = new Map(
      d.members.map((member) => [
        member.id,
        {
          x: limit(member.x + d.nextX - d.x),
          y: limit(member.y + d.nextY - d.y),
        },
      ]),
    );
    edit(
      local.current.map((node) =>
        moved.has(node.id) ? { ...node, ...moved.get(node.id)! } : node,
      ),
    );
  }
  async function readConflict() {
    const controller = new AbortController();
    request.current = controller;
    try {
      const server = requireCanvas(
        await getCanvas({ id: base.id }, { signal: controller.signal }),
        base.project_id,
        base.id,
      );
      if (alive.current) setConflict({ server });
    } catch {
      if (alive.current && !controller.signal.aborted)
        setMessage("未能读取服务端版本，本地编辑仍保留，请重试读取。");
    }
  }
  const saveFlight = useRef<Promise<Canvas | undefined> | null>(null);
  const saveLatest = useRef<() => Promise<Canvas | undefined>>(
    async () => undefined,
  );
  async function save() {
    if (saveFlight.current) return saveFlight.current;
    const flight = (async () => {
      let result = await saveOnce();
      while (
        result &&
        alive.current &&
        !privateInvalid.current &&
        commandBatch(result, local.current).commands?.length
      ) {
        result = await saveOnce();
      }
      return result;
    })();
    saveFlight.current = flight;
    try {
      return await flight;
    } finally {
      saveFlight.current = null;
    }
  }
  async function saveOnce() {
    if (busy.current || conflict || privateInvalid.current) return;
    const base = savedBase.current;
    let operation = pending.current;
    if (!operation) {
      try {
        const frozen = structuredClone(local.current);
        const body = commandBatch(base, frozen);
        if (!body.commands?.length) return base;
        operation = { key: crypto.randomUUID(), base, nodes: frozen, body };
        pending.current = operation;
      } catch {
        setMessage(
          "请检查节点名称、提示词和参数；本次最多提交 100 条编辑命令。",
        );
        return;
      }
    }
    busy.current = true;
    setSaving(true);
    setMessage("");
    await persistCanvas();
    const controller = new AbortController();
    request.current = controller;
    try {
      const value = await applyCanvasCommands({ id: base.id }, operation.body, {
        headers: { "Idempotency-Key": operation.key },
        signal: controller.signal,
      });
      const committed = requireSaveResult(
        value,
        operation.base,
        operation.nodes,
        operation.body,
      );
      if (!alive.current || controller.signal.aborted) return;
      const frozen = new Map(operation.nodes.map((n) => [n.id, n]));
      const saved = new Map(committed.nodes.map((n) => [n.id, n]));
      edit(
        local.current.map((n) => {
          const original = frozen.get(n.id),
            receipt = saved.get(n.id);
          if (!receipt || !original) return n;
          return sameEditable(n, original)
            ? receipt
            : { ...n, input_revision: receipt.input_revision };
        }),
      );
      setBase(committed);
      pending.current = null;
      setUnconfirmed(false);
      return committed;
    } catch (error) {
      if (!alive.current || controller.signal.aborted) return;
      if (error instanceof RequestError && error.code === "revision_conflict") {
        pending.current = null;
        setUnconfirmed(false);
        setConflict({ server: null });
        setMessage(
          "画布已被其他窗口修改，本地编辑已保留。请对照服务端内容后明确选择。",
        );
        await readConflict();
      } else if (
        error instanceof RequestError &&
        [401, 403, 404, 409, 422].includes(error.status)
      ) {
        pending.current = null;
        setUnconfirmed(false);
        setMessage(
          error.status === 401
            ? "登录已失效，本地编辑仍保留，请重新登录后重开画布。"
            : "服务端拒绝保存，本地编辑仍保留。请检查权限、节点及参数。",
        );
      } else {
        setUnconfirmed(true);
        setMessage(
          "保存结果未确认，本地编辑仍保留。请重试原保存，避免重复提交。",
        );
      }
    } finally {
      busy.current = false;
      if (alive.current) setSaving(false);
    }
  }
  useEffect(() => {
    saveLatest.current = save;
  });
  useEffect(() => {
    if (!dirty || saving || unconfirmed || conflict || identityInvalid) return;
    const timer = setTimeout(() => {
      void saveLatest.current();
    }, 900);
    return () => clearTimeout(timer);
  }, [nodes, dirty, saving, unconfirmed, conflict, identityInvalid]);
  async function recoverCanvas(branch: LocalBranch) {
    if (busy.current || privateInvalid.current) return;
    if (pending.current) {
      setMessage("请先重试原保存，确认结果后再恢复其他分支。");
      return;
    }
    const expectedNodes = local.current;
    busy.current = true;
    setSaving(true);
    try {
      const value = branch.ownerState as {
        base: Canvas;
        nodes: Node[];
        pending: Pending | null;
      };
      let baseline = requireCanvas(value.base, project.id, initial.id);
      commandBatch(baseline, value.nodes);
      if (value.pending) {
        if (
          canonical(value.pending.body) !==
          canonical(commandBatch(value.pending.base, value.pending.nodes))
        )
          throw new Error("恢复请求不完整");
        const replay = await applyCanvasCommands(
          { id: initial.id },
          value.pending.body,
          { headers: { "Idempotency-Key": value.pending.key } },
        );
        baseline = requireSaveResult(
          replay,
          value.pending.base,
          value.pending.nodes,
          value.pending.body,
        );
      }
      const server = requireCanvas(
        await getCanvas({ id: initial.id }),
        project.id,
        initial.id,
      );
      if (!alive.current || privateInvalid.current) return;
      if (local.current !== expectedNodes)
        throw new Error("恢复期间有新编辑，未覆盖；原分支仍保留。");
      edit(value.nodes);
      if (server.revision !== baseline.revision) {
        setConflict({ server });
        setMessage("已恢复所选本地分支，请处理服务端冲突。");
      } else setBase(server);
    } catch (e) {
      if (alive.current && !privateInvalid.current)
        setMessage(
          e instanceof Error ? e.message : "本地恢复失败；原分支仍保留。",
        );
    } finally {
      busy.current = false;
      if (alive.current) setSaving(false);
    }
  }
  async function generateCurrent(pickSubject = false) {
    if (!current || !isDraft(current) || busy.current) return;
    const saved = await save();
    if (!saved) return;
    const node = saved.nodes.find((n) => n.id === current.id);
    if (!node || !isDraft(node)) return;
    let composer = node.config.draft.composer;
    if (!composer) {
      composer = emptyPromptDraft();
      composer.document = {
        type: "doc",
        content: node.config.draft.prompt.split("\n").map((text) => ({
          type: "paragraph",
          ...(text ? { content: [{ type: "text" as const, text }] } : {}),
        })),
      };
      composer.params = {
        aspect_ratio: node.config.draft.aspect_ratio,
        output_count: node.config.draft.candidate_count,
        ...(node.config.draft.negative_prompt
          ? { negative_prompt: node.config.draft.negative_prompt }
          : {}),
      };
    }
    const ownerNode = node.id;
    setGeneration({
      project,
      prompt: "",
      composer,
      capability: node.node_type === "video_generation" ? "video" : "image",
      pickSubject,
      source: {
        canvas_id: saved.id,
        node_id: ownerNode,
        revision: saved.revision,
      },
      canvasOwner: {
        contextKey: `canvas/${saved.id}/${ownerNode}`,
        draft: composer,
        onChange: (value: PromptDraft) => {
          const n = local.current.find((n) => n.id === ownerNode);
          if (n && isDraft(n))
            update(ownerNode, { config: { draft: richCanvasDraft(value) } });
        },
        flush: async () => {
          const committed = await saveLatest.current();
          if (!committed)
            throw new Error("画布保存尚未确认，请先处理冲突或重试原保存。");
          const owner = committed.nodes.find((n) => n.id === ownerNode);
          if (!owner || !isDraft(owner) || !owner.config.draft.composer)
            throw new Error("请先完成节点草稿保存。");
          const latest = local.current.find((n) => n.id === ownerNode);
          if (!latest || !sameEditable(latest, owner))
            throw new Error("保存期间节点已变化，请重新保存当前输入。");
          return {
            kind: "canvas_input",
            canvas_id: committed.id,
            node_id: ownerNode,
            canvas_revision: committed.revision,
            input_revision: owner.input_revision,
          };
        },
      },
    });
  }

  async function adoptResult(id: string, kind: string) {
    if (!["image", "video", "audio"].includes(kind) || busy.current || conflict)
      throw new Error("请先解决画布保存状态，再采用结果。");
    const existing = local.current.find(
      (n) => n.ref_type === "media_asset" && n.ref_id === id,
    );
    if (!existing) {
      const node: Node = {
        id: crypto.randomUUID(),
        title: kind === "video" ? "生成视频" : "生成图片",
        node_type: kind,
        node_action: "resource",
        ref_type: "media_asset",
        ref_id: id,
        config: {},
        x: 80 + (local.current.length % 3) * 340,
        y: 80 + Math.floor(local.current.length / 3) * 260,
        z_index: 0,
        width: 288,
        height: 200,
      };
      edit([...local.current, node]);
      selectNode(node.id);
    } else selectNode(existing.id);
    const saved = await save();
    if (!saved)
      throw new Error(
        "结果已保留在本地画布，保存尚未确认。请重试采用以继续同一次保存。",
      );
    setGeneration(null);
    setMessage("生成结果已添加并保存到画布。");
  }
  const saveShortcut = useEffectEvent(() => {
    void save();
  });
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        saveShortcut();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  function resolveConflict(discard: boolean) {
    const server = conflict?.server;
    if (!server) return;
    if (discard) {
      edit(server.nodes);
      selectNode(undefined);
    } else {
      const own = new Map(local.current.map((n) => [n.id, n]));
      const previous = new Map(base.nodes.map((n) => [n.id, n]));
      const merged = server.nodes.map((n) => {
        const ours = own.get(n.id),
          old = previous.get(n.id);
        return ours && old && !sameEditable(ours, old)
          ? { ...ours, input_revision: n.input_revision }
          : n;
      });
      const remoteIDs = new Set(server.nodes.map((n) => n.id));
      for (const n of local.current) if (!remoteIDs.has(n.id)) merged.push(n);
      edit(merged);
    }
    setBase(server);
    setConflict(null);
    setMessage("");
  }
  const status = saving
    ? "正在保存…"
    : unconfirmed
      ? "保存未确认"
      : conflict
        ? "版本冲突，本地编辑保留"
        : dirty
          ? "有未保存的编辑"
          : `已保存 · 版本 ${base.revision}`;
  if (identityInvalid)
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>请重新打开画布</EmptyTitle>
          <EmptyDescription>登录状态已变化，本地编辑已保留。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button asChild>
            <Link href="/login">重新登录</Link>
          </Button>
        </EmptyContent>
      </Empty>
    );
  return (
    <main className="creation-canvas relative h-dvh overflow-hidden">
      <header
        className="canvas-header"
        onClickCapture={(e) => {
          if (
            (dirty || saving || unconfirmed) &&
            e.target instanceof Element &&
            e.target.closest('a[href="/projects"],a[href="/"]')
          ) {
            e.preventDefault();
            setLeave(true);
          }
        }}
      >
        <div className="canvas-topbar-cluster max-w-[65%] min-w-0">
          <Button asChild variant="ghost" size="icon-sm">
            <Link
              href="/projects"
              onClick={(e) => {
                if (dirty || saving || unconfirmed) {
                  e.preventDefault();
                  setLeave(true);
                }
              }}
            >
              <ArrowLeft data-icon="inline-start" />
              <span className="sr-only">返回项目</span>
            </Link>
          </Button>
          <Brand wordmark />
          <div className="min-w-0 flex-1">
            <h1 className="truncate font-medium">{project.name}</h1>
            <p className="truncate text-xs text-muted-foreground">
              {project.aspect_ratio} · {project.style_key ?? "未设风格"}
            </p>
          </div>
        </div>
        <div className="canvas-topbar-cluster shrink-0">
          <span
            role="status"
            aria-live="polite"
            className="text-xs text-muted-foreground"
          >
            {saving ? (
              <Spinner aria-hidden="true" />
            ) : dirty || unconfirmed || conflict ? (
              <CircleAlert />
            ) : (
              <Check />
            )}
            {status}
          </span>
          <Button
            onClick={() => void save()}
            disabled={saving || !!conflict || (!dirty && !unconfirmed)}
          >
            {saving ? (
              <Spinner data-icon="inline-start" aria-hidden="true" />
            ) : (
              <Save data-icon="inline-start" />
            )}
            {saving ? "正在保存…" : unconfirmed ? "重试原保存" : "保存画布"}
          </Button>
        </div>
      </header>
      {message && <FeedbackToast message={message} tone="info" />}
      <Dialog open={leave} onOpenChange={setLeave}>
        <DialogContent role="alertdialog" showCloseButton={false}>
          <DialogTitle>确认离开画布</DialogTitle>
          <DialogDescription>
            未保存的编辑在离开后会丢失；未确认保存可能已写入，请重开后核对实际结果。
          </DialogDescription>
          <DialogFooter>
            <Button variant="outline" onClick={() => setLeave(false)}>
              留在画布
            </Button>
            <Button asChild variant="destructive">
              <Link
                href="/projects"
                onClick={() => {
                  leaving.current = true;
                }}
              >
                放弃未保存编辑并返回项目库
              </Link>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {conflict && (
        <section
          aria-label="版本冲突处理"
          className="canvas-feedback flex flex-col gap-3 text-sm"
        >
          <p>
            本地基于版本 {base.revision}；
            {conflict.server
              ? `服务端版本 ${conflict.server.revision}`
              : "正在读取服务端版本…"}
            。本地稿仍在编辑面板中。
          </p>
          {conflict.server && (
            <div className="max-h-40 overflow-auto rounded border p-3">
              <p className="mb-2 font-medium">服务端草稿</p>
              {conflict.server.nodes.filter(isDraft).map((n) => (
                <p key={n.id} className="break-words whitespace-pre-wrap">
                  {n.title} · {n.config.draft.aspect_ratio} ·{" "}
                  {n.config.draft.candidate_count} 个候选
                  <br />
                  {n.config.draft.prompt || "（空提示词）"}
                  <br />
                  {n.config.draft.negative_prompt}
                </p>
              ))}
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {conflict.server ? (
              <>
                <Button
                  variant="outline"
                  onClick={() => resolveConflict(false)}
                >
                  保留本地编辑，以版本 {conflict.server.revision} 继续
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => resolveConflict(true)}
                >
                  放弃本地编辑，载入服务端版本
                </Button>
              </>
            ) : (
              <Button variant="outline" onClick={() => void readConflict()}>
                重试读取服务端版本
              </Button>
            )}
          </div>
        </section>
      )}
      <InfiniteCanvas
        containerRef={board}
        viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
        onViewportChange={viewportChange}
        onViewportPreviewChange={viewportPreview}
        backgroundMode="dots"
        boxSelectEnabled={tool === "select"}
        onSelectArea={selectArea}
        onCanvasDeselect={() => selectNode(undefined)}
      >
        {nodes.map((node) => (
          <CanvasNode
            key={node.id}
            node={node}
            projectID={project.id}
            selected={selectedIDs.includes(node.id)}
            zoom={camera.zoom}
            onResize={(size) =>
              update(node.id, { ...size, x: limit(size.x), y: limit(size.y) })
            }
            onResizePreview={() =>
              board.current?.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT))
            }
            onSelect={() => {
              if (selectedIDs.includes(node.id)) setSelected(node.id);
              else selectNode(node.id);
            }}
            onEditTitle={() => {
              selectNode(node.id);
              setDetails(true);
            }}
            onMove={(x, y) => update(node.id, { x: limit(x), y: limit(y) })}
            onDragStart={(event) => beginDrag(event, node)}
            onDragMove={moveDrag}
            onDragEnd={endDrag}
            onDuplicate={() => copyNode(node)}
            canRemove={
              !baseline.has(node.id) && !saving && !unconfirmed && !conflict
            }
            onRemove={() => removeLocal(node)}
          />
        ))}
      </InfiniteCanvas>
      {!nodes.length && (
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Plus />
              </EmptyMedia>
              <EmptyTitle>画布暂无节点</EmptyTitle>
              <EmptyDescription>
                从左侧添加图片、视频节点或项目素材，开始你的创作。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </div>
      )}
      {selectedIDs.length > 1 && (
        <p className="pointer-events-none absolute top-20 left-4 rounded-lg bg-card px-3 py-2 text-xs">
          已选择 {selectedIDs.length} 个节点；拖动标题可移动可编辑节点
        </p>
      )}
      <CanvasToolbar
        tool={tool}
        onToolChange={setTool}
        onAdd={add}
        onMaterials={(tab = "assets", kind) => setMaterials({ tab, kind })}
        onUpload={() => setUpload(true)}
        zoom={camera.zoom}
        onZoom={(zoom) => changeCamera({ ...view.current, zoom })}
        onFit={fit}
        miniMap={miniMap}
        onMiniMap={() => setMiniMap(!miniMap)}
      />
      {miniMap && (
        <CanvasMiniMap
          nodes={nodes}
          viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
          width={size.width}
          height={size.height}
          onNavigate={(x, y) =>
            changeCamera({
              x: size.width / 2 - x * view.current.zoom,
              y: size.height / 2 - y * view.current.zoom,
              zoom: view.current.zoom,
            })
          }
        />
      )}
      {current && (
        <CanvasNodePanel
          node={current}
          viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
          containerRef={board}
        >
          <CanvasComposer
            node={current}
            nodes={nodes}
            details={details}
            onDetails={setDetails}
            onSelect={selectNode}
            onUpdate={(patch) => update(current.id, patch)}
            onDraft={draft}
            onGenerate={() => void generateCurrent()}
            onSubject={() => void generateCurrent(true)}
            onPreview={() => setPreview(current)}
            onContinue={() => {
              if (
                current.ref_id &&
                ["image", "video", "audio"].includes(current.node_type)
              )
                setGeneration({
                  project,
                  prompt: "",
                  capability: current.node_type === "image" ? "image" : "video",
                  reference: {
                    assetID: current.ref_id,
                    kind: current.node_type as "image" | "video" | "audio",
                  },
                });
            }}
            generating={saving}
            blocked={!!conflict || unconfirmed}
          />
        </CanvasNodePanel>
      )}
      {recoveryBranches.length > 0 && (
        <div
          className="fixed bottom-4 left-4 z-40 max-w-lg space-y-2 rounded-lg border bg-background p-4"
          role="status"
        >
          <p>发现 {recoveryBranches.length} 份未同步画布分支</p>
          {recoveryBranches.map((branch) => (
            <div key={branch.replicaId} className="flex gap-2">
              <span>基线 r{branch.serverRevision}</span>
              <Button
                disabled={saving}
                onClick={() => void recoverCanvas(branch)}
              >
                恢复此分支
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  void branchJournal.remove(branch).then((removed) => {
                    if (removed)
                      setRecoveryBranches((v) =>
                        v.filter((b) => b.replicaId !== branch.replicaId),
                      );
                    else setMessage("该分支已有新修改，未删除。");
                  });
                }}
              >
                丢弃此分支
              </Button>
            </div>
          ))}
        </div>
      )}
      {generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
          onAdopt={adoptResult}
        />
      )}
      {materials && (
        <CanvasMaterialDialog
          projectID={project.id}
          initialTab={materials.tab}
          initialKind={materials.kind}
          onClose={() => setMaterials(null)}
          onAdd={addMaterials}
          onSubject={useSubject}
        />
      )}
      {upload && (
        <UploadDialog
          projectID={project.id}
          onClose={() => setUpload(false)}
          onUploaded={(asset) => {
            setUpload(false);
            if (
              asset &&
              ["image", "video", "audio", "model"].includes(asset.kind ?? "")
            )
              addMaterials([asset]);
            else
              setMessage(
                "文件已上传。当前画布支持图片、视频、音频及 3D 素材，请在项目素材中查看文件。",
              );
          }}
        />
      )}
      {preview?.ref_id && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setPreview(null);
          }}
        >
          <DialogContent className="sm:max-w-4xl">
            <DialogTitle>{preview.title}</DialogTitle>
            <DialogDescription>当前项目的素材预览</DialogDescription>
            <div className="relative flex h-[min(60dvh,560px)] items-center justify-center overflow-hidden rounded-lg">
              <ProjectMediaPreview
                projectID={project.id}
                id={preview.ref_id}
                kind={preview.node_type}
                large
              />
            </div>
          </DialogContent>
        </Dialog>
      )}
    </main>
  );
}

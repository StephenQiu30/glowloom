/** Maintained E0 browser fixture, served by Vite only; never a Next.js product route. */
import React, { useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { PromptEditor } from "../../src/components/generation/prompt-editor";
import {
  emptyPromptDraft,
  type ReferenceBinding,
} from "../../src/components/generation/prompt-document";
import {
  type ComposerSession,
  type ComposerSnapshot,
  type ModeRequest,
} from "../../src/components/generation/composer-session";

const performanceMode = new URLSearchParams(location.search).has("performance");
function initialDraft() {
  const draft = emptyPromptDraft();
  if (!performanceMode) return draft;
  draft.references = Array.from({ length: 50 }, (_, i) => ({
    id: `perf-${i}`,
    bindingGeneration: 1,
    mediaKind: "image",
    role: "reference_image",
    order: i,
    source: { kind: "media", sourceId: `asset-${i}`, sourceVersion: "1" },
  }));
  draft.labelState.highWater.image = 50;
  for (let i = 0; i < 50; i++)
    draft.labelState.allocations[`perf-${i}`] = {
      labelKind: "image",
      ordinal: i + 1,
    };
  draft.document.content[0].content = [
    { type: "text", text: "汉字🐈测试".repeat(2000) },
    ...Array.from({ length: 200 }, (_, i) => ({
      type: "reference" as const,
      attrs: { referenceId: `perf-${i % 50}` },
    })),
  ];
  return draft;
}
function Fixture() {
  const session = useRef<ComposerSession | null>(null);
  const counter = useRef(0);
  const pending = useRef<ModeRequest | null>(null);
  const frozen = useRef<ComposerSnapshot | null>(null);
  const [snapshot, setSnapshot] = useState<ComposerSnapshot | null>(null);
  const [saved, setSaved] = useState("");
  const [measurement, setMeasurement] = useState("");
  const [seed, setSeed] = useState(initialDraft);
  const [replicaId, setReplicaId] = useState("browser-1");
  const publish = () => setSnapshot(session.current?.snapshot() ?? null);
  const insert = (role = "reference_image", duplicate = false) => {
    const binding: ReferenceBinding = {
      id: `reference-${++counter.current}`,
      role,
      mediaKind: "image",
      order: 0,
      bindingGeneration: 1,
      source: {
        kind: "media",
        sourceId: duplicate ? "asset-1" : `asset-${counter.current}`,
        sourceVersion: "v1",
      },
    };
    session.current?.insertReference(binding);
  };
  return (
    <main
      style={{
        maxWidth: 900,
        margin: "24px auto",
        padding: 16,
        fontFamily: "sans-serif",
      }}
    >
      <h1>E0 编辑器回归夹具</h1>
      <p>
        仅验证编辑内核。快照保存在本页内存；没有服务端保存、素材授权或生成请求。
      </p>
      <div
        style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 16 }}
      >
        <button onClick={() => insert()}>添加图片</button>
        <button onClick={() => insert("reference_image", true)}>
          再次提及图片1
        </button>
        <button onClick={() => insert("last_frame", true)}>同图作为尾帧</button>
        <button
          onClick={() => {
            const id = session.current?.snapshot().draft.references[0]?.id;
            if (id) session.current?.removeReference(id);
          }}
        >
          移除第一份参考
        </button>
        <button onClick={() => session.current?.undo()}>撤销</button>
        <button onClick={() => session.current?.redo()}>重做</button>
        <button
          onClick={() => {
            const s = session.current;
            if (s)
              s.resolveModeSelection(
                s.beginModeSelection({
                  mode: "omni",
                  modelVersion: "fixture-v1",
                }),
              );
          }}
        >
          切全能参考
        </button>
        <button
          onClick={() => {
            pending.current =
              session.current?.beginModeSelection({
                mode: "text",
                modelVersion: "fixture-v1",
              }) ?? null;
          }}
        >
          延迟切纯文本
        </button>
        <button
          onClick={() => {
            if (pending.current)
              session.current?.resolveModeSelection(pending.current);
          }}
        >
          释放模式回执
        </button>
        <button
          onClick={() => {
            frozen.current = session.current?.snapshot() ?? null;
            setSaved(JSON.stringify(frozen.current));
          }}
        >
          冻结快照
        </button>
        <button
          onClick={() => {
            if (frozen.current) {
              setSeed(frozen.current.draft);
              setReplicaId(`browser-${++counter.current}`);
            }
          }}
        >
          从冻结快照重建
        </button>
        <button onClick={publish}>读取当前快照</button>
        {performanceMode && (
          <button
            onClick={() =>
              void (async () => {
                const samples: number[] = [];
                for (let i = 0; i < 30; i++) {
                  const start = performance.now();
                  session.current!.editor.commands.insertContent("中");
                  await new Promise<void>((resolve) =>
                    requestAnimationFrame(() =>
                      requestAnimationFrame(() => resolve()),
                    ),
                  );
                  samples.push(performance.now() - start);
                }
                samples.sort((a, b) => a - b);
                setMeasurement(
                  JSON.stringify({
                    kind: "programmatic-edit-to-two-animation-frames",
                    samples: samples.length,
                    p95ms: samples[Math.ceil(samples.length * 0.95) - 1],
                    maxms: samples.at(-1),
                    textCodePoints: 10000,
                    references: 50,
                    mentions: 200,
                  }),
                );
              })()
            }
          >
            运行编辑绘制采样
          </button>
        )}
      </div>
      <PromptEditor
        contextKey="fixture-account/project/draft"
        replicaId={replicaId}
        draft={seed}
        onReady={(s) => {
          session.current = s;
          s.editor.on("transaction", publish);
          publish();
        }}
        onRequestSave={(value) => {
          frozen.current = value;
          setSaved(JSON.stringify(value));
        }}
      />
      <p>
        生效模式：<output id="mode">{snapshot?.draft.mode ?? "未选择"}</output>
        ；参考数：
        <output id="count">{snapshot?.draft.references.length ?? 0}</output>
      </p>
      <output id="measurement">{measurement}</output>
      <h2>当前完整快照</h2>
      <pre
        id="current"
        style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}
      >
        {performanceMode
          ? "性能模式省略调试 JSON"
          : JSON.stringify(snapshot, null, 2)}
      </pre>
      <h2>冻结快照（本页内存）</h2>
      <pre
        id="frozen"
        style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}
      >
        {saved}
      </pre>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);

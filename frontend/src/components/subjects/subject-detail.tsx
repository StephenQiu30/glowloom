"use client";
import { useRef, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { AuthError } from "@/components/auth/request";
import { flowError } from "@/components/generation/contracts";
import {
  deleteBibleEntry,
  restoreBibleEntry,
  confirmBibleEntry,
  listBibleVersions,
  getBibleEntry,
} from "@/components/creation/generated/creation";
import {
  subjectImage,
  subjectText,
  requireSubjectReceipt,
  requireSubject,
  impactProof,
  type Subject,
} from "./contracts";
import { SubjectEditor } from "./subject-editor";
export function SubjectDetail({
  subject,
  onClose,
  onChanged,
  onUse,
}: {
  subject: Subject;
  onClose: () => void;
  onChanged: (s: Subject) => void;
  onUse?: (s: Subject) => void;
}) {
  const [value, setValue] = useState(subject),
    [edit, setEdit] = useState(false),
    [action, setAction] = useState<"delete" | "restore" | "confirm" | null>(
      null,
    ),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [uncertain, setUncertain] = useState(false),
    [stale, setStale] = useState(false),
    [proof, setProof] = useState<CreationAPI.ImpactProof>(),
    [history, setHistory] = useState<CreationAPI.Version[] | null>(null),
    [cursor, setCursor] = useState<string | undefined>(),
    [historyPending, setHistoryPending] = useState(false);
  const op = useRef<{ key: string; body: CreationAPI.ControlRequest } | null>(
      null,
    ),
    busy = useRef(false);
  const params = {
    pid: value.head.project_id,
    kind: value.head.kind,
    id: value.head.id,
  };
  function changed(s: Subject) {
    setValue(s);
    onChanged(s);
  }
  async function control() {
    if (!action || busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    const t = op.current ?? {
      key: crypto.randomUUID(),
      body: {
        expected_revision: value.head.revision,
        ...(proof ? { acknowledged_impact: proof } : {}),
      },
    };
    op.current = t;
    try {
      const fn =
        action === "delete"
          ? deleteBibleEntry
          : action === "restore"
            ? restoreBibleEntry
            : confirmBibleEntry;
      const receipt = requireSubjectReceipt(
        await fn(params, t.body, { headers: { "Idempotency-Key": t.key } }),
        value.head.kind,
        value.head.id,
      );
      if (receipt.revision !== t.body.expected_revision! + 1)
        throw new AuthError("invalid_receipt");
      const s = requireSubject(
        await getBibleEntry(params),
        params.pid,
        params.kind,
        params.id,
      );
      if (
        s.head.revision < receipt.revision ||
        (s.head.revision === receipt.revision &&
          (s.head.current_version_id !== receipt.version_id ||
            (action === "delete" && !s.head.deleted) ||
            (action === "restore" && s.head.deleted) ||
            (action === "confirm" &&
              s.head.confirmed_version_id !== s.current.id)))
      )
        throw new AuthError("invalid_receipt");
      changed(s);
      if (s.head.revision > receipt.revision)
        setError("操作已完成，主体随后又有更新。当前显示的是最新状态。");
      setAction(null);
      setProof(undefined);
      setUncertain(false);
      op.current = null;
    } catch (e) {
      const impact =
        e instanceof AuthError && e.code === "impact_acknowledgment_required"
          ? impactProof(e.meta)
          : undefined;
      if (impact) {
        setError(proof ? "引用已发生变化，请核对最新影响后再删除。" : "");
        setProof(impact);
        setUncertain(false);
        op.current = null;
      } else {
        const conflict =
          e instanceof AuthError && e.code === "revision_conflict";
        setStale(conflict);
        setError(
          conflict
            ? "主体已在其他窗口更新，请刷新后重新确认操作。"
            : flowError(e),
        );
        const unknown = !(
          e instanceof AuthError &&
          e.status >= 400 &&
          e.status < 500
        );
        setUncertain(unknown);
        if (!unknown) op.current = null;
      }
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  async function versions(next?: string) {
    if (historyPending) return;
    setHistoryPending(true);
    setError("");
    try {
      const r = await listBibleVersions({ ...params, limit: 20, cursor: next });
      if (
        !Array.isArray(r.versions) ||
        r.versions.some(
          (v) => v.entry_id !== params.id || v.project_id !== params.pid,
        )
      )
        throw new AuthError("invalid_receipt");
      setHistory((h) => (next ? [...(h ?? []), ...r.versions!] : r.versions!));
      setCursor(r.next_cursor);
    } catch (e) {
      setError(flowError(e));
    } finally {
      setHistoryPending(false);
    }
  }
  async function refresh() {
    setError("");
    try {
      changed(
        requireSubject(
          await getBibleEntry(params),
          params.pid,
          params.kind,
          params.id,
        ),
      );
      if (!uncertain) {
        setStale(false);
        setAction(null);
        setProof(undefined);
        op.current = null;
      }
    } catch (e) {
      setError(flowError(e));
    }
  }
  const text = subjectText(value),
    image = subjectImage(value);
  function cancelAction() {
    if (pending || uncertain) return;
    setAction(null);
    setProof(undefined);
    setError("");
    setStale(false);
    op.current = null;
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(v) => {
          if (!v && !pending && !uncertain) {
            if (action) cancelAction();
            else onClose();
          }
        }}
      >
        <DialogContent
          className={
            action ? "subject-control" : "subject-detail sm:max-w-[680px]"
          }
          showCloseButton={!pending && !uncertain}
          overlayClassName={action ? "bg-black/65" : undefined}
          onInteractOutside={(e) => e.preventDefault()}
        >
          {!action && (
            <>
              <DialogTitle>{text.name}</DialogTitle>
              <DialogDescription>
                版本 {value.current.number} ·{" "}
                {value.head.deleted
                  ? "已删除"
                  : value.head.confirmed_version_id === value.current.id
                    ? "已确认"
                    : "待确认"}
              </DialogDescription>
              {image && (
                <div className="subject-detail-image">
                  <ProjectMediaPreview
                    projectID={params.pid}
                    id={image}
                    large
                  />
                </div>
              )}
              <p className="break-words whitespace-pre-wrap">
                {text.description || "还没有描述。"}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  disabled={pending || uncertain || !!value.head.deleted}
                  onClick={() => setEdit(true)}
                >
                  编辑主体
                </Button>
                {onUse && (
                  <Button
                    disabled={!!value.head.deleted || uncertain}
                    onClick={() => onUse(value)}
                  >
                    用于生成
                  </Button>
                )}
                <Button
                  variant="outline"
                  disabled={pending || uncertain}
                  onClick={() => {
                    setError("");
                    setAction(value.head.deleted ? "restore" : "delete");
                  }}
                >
                  {" "}
                  {value.head.deleted ? "恢复主体" : "删除主体"}
                </Button>
                {!value.head.deleted &&
                  value.head.confirmed_version_id !== value.current.id && (
                    <Button
                      variant="outline"
                      disabled={pending || uncertain}
                      onClick={() => setAction("confirm")}
                    >
                      确认当前版本
                    </Button>
                  )}
                <Button
                  variant="ghost"
                  disabled={historyPending}
                  onClick={() => void versions()}
                >
                  {historyPending ? "正在读取…" : "版本历史"}
                </Button>
                <Button
                  variant="ghost"
                  disabled={pending}
                  onClick={() => void refresh()}
                >
                  刷新
                </Button>
              </div>
              {error && (
                <p role="alert" className="flow-error">
                  {error}
                </p>
              )}
            </>
          )}
          {action && (
            <>
              <DialogTitle>
                <span className="subject-control-copy">
                  {action === "delete"
                    ? `删除“${text.name}”？`
                    : action === "restore"
                      ? `恢复“${text.name}”？`
                      : "确认当前主体版本？"}
                </span>
              </DialogTitle>
              <DialogDescription>
                <span className="subject-control-copy">
                  {action === "delete"
                    ? "已生成作品不受影响。新创作需要重新选择主体。"
                    : action === "restore"
                      ? "恢复后可继续编辑和引用，历史版本与确认记录保留。"
                      : "将当前内容设为可引用的确认版本。"}
                </span>
              </DialogDescription>
              <div className="subject-control-body" aria-live="polite">
                {proof && (
                  <p>
                    {proof.current_lines === 0 &&
                    proof.historical_lines === 0 &&
                    proof.aliases === 0
                      ? "已核对：没有关联的剧本引用。"
                      : `当前台词 ${proof.current_lines} 处 · 历史台词 ${proof.historical_lines} 处 · 合并别名 ${proof.aliases} 个。历史内容保留，新引用需重新选择。`}
                  </p>
                )}
                {action === "delete" && <p>删除后可在“已删除”中恢复。</p>}
                {error && (
                  <p role="alert" className="flow-error">
                    {error}
                  </p>
                )}
                {error && !uncertain && (
                  <Button
                    variant="link"
                    disabled={pending}
                    onClick={() => void refresh()}
                  >
                    刷新主体后重试
                  </Button>
                )}
                {uncertain && (
                  <p>尚未确认操作结果。重试会核对同一请求，不会重复执行。</p>
                )}
              </div>
              <div className="subject-control-actions">
                <Button
                  variant="outline"
                  disabled={pending || uncertain}
                  onClick={cancelAction}
                >
                  取消
                </Button>
                <Button
                  variant="default"
                  disabled={pending || stale}
                  onClick={() => void control()}
                >
                  {pending
                    ? "正在处理…"
                    : uncertain
                      ? "重试原操作"
                      : proof
                        ? "删除主体"
                        : action === "delete"
                          ? "检查引用"
                          : action === "restore"
                            ? "恢复主体"
                            : "确认"}
                </Button>
              </div>
            </>
          )}
          {!action && history && (
            <section className="subject-history">
              <h3>版本历史</h3>
              {history.map((v) => (
                <article key={v.id}>
                  <strong>
                    版本 {v.number} · {v[v.kind!]?.name}
                  </strong>
                  <small>
                    {v.created_at
                      ? new Date(v.created_at).toLocaleString("zh-CN")
                      : ""}
                  </small>
                  <p>{v[v.kind!]?.description || "无描述"}</p>
                </article>
              ))}
              {cursor && (
                <Button
                  variant="ghost"
                  disabled={historyPending}
                  onClick={() => void versions(cursor)}
                >
                  更多版本
                </Button>
              )}
            </section>
          )}
        </DialogContent>
      </Dialog>
      {edit && (
        <SubjectEditor
          projectID={params.pid}
          kind={params.kind}
          initial={value}
          onClose={() => setEdit(false)}
          onSaved={(s) => {
            changed(s);
            setEdit(false);
          }}
        />
      )}
    </>
  );
}

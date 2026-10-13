"use client";
import { useRef, useState } from "react";
import { ImagePlus } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  ProjectMediaPicker,
  ProjectMediaPreview,
} from "@/components/media/project-media";
import {
  createBibleEntry,
  updateBibleEntry,
  getBibleEntry,
  replaceBibleReferences,
} from "@/components/creation/generated/creation";
import { AuthError } from "@/components/auth/request";
import { flowError } from "@/components/generation/contracts";
import { isProjectID } from "@/components/projects/project";
import {
  kinds,
  requireSubject,
  requireSubjectReceipt,
  subjectImage,
  subjectText,
  type Subject,
} from "./contracts";
export function SubjectEditor({
  projectID,
  kind,
  initial,
  onClose,
  onSaved,
}: {
  projectID: string;
  kind: CreationAPI.Kind;
  initial?: Subject;
  onClose: () => void;
  onSaved: (s: Subject) => void;
}) {
  const [name, setName] = useState(initial ? subjectText(initial).name! : "");
  const [description, setDescription] = useState(
    initial ? (subjectText(initial).description ?? "") : "",
  );
  const [image, setImage] = useState(
      initial ? subjectImage(initial) : undefined,
    ),
    [picker, setPicker] = useState(false),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [frozen, setFrozen] = useState(false),
    [partial, setPartial] = useState(false),
    [leave, setLeave] = useState(false);
  const busy = useRef(false),
    transaction = useRef<{
      key: string;
      body: CreationAPI.ContentRequest;
      id?: string;
      revision?: number;
      metadataDone: boolean;
      referenceKey: string;
      referenceBody?: CreationAPI.ReferencesRequest;
      look?: string;
    } | null>(null);
  const dirty =
    name !== (initial ? subjectText(initial).name : "") ||
    description !== (initial ? (subjectText(initial).description ?? "") : "") ||
    image !== (initial ? subjectImage(initial) : undefined);
  function close() {
    if (pending) return;
    if (frozen || dirty) setLeave(true);
    else onClose();
  }
  async function save() {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    setFrozen(true);
    const old = initial ? subjectText(initial) : {};
    const t = transaction.current ?? {
      key: crypto.randomUUID(),
      body: {
        expected_revision: initial?.head.revision ?? 0,
        [kind]: {
          ...(kind === "character"
            ? {
                aliases: old.aliases,
                definition: initial?.current.character?.definition,
              }
            : old),
          name: name.trim(),
          description: description.trim(),
        },
      },
      id: initial?.head.id,
      metadataDone: false,
      referenceKey: crypto.randomUUID(),
    };
    transaction.current = t;
    try {
      if (!t.metadataDone) {
        const result = t.id
          ? await updateBibleEntry({ pid: projectID, kind, id: t.id }, t.body, {
              headers: { "Idempotency-Key": t.key },
            })
          : await createBibleEntry({ pid: projectID, kind }, t.body, {
              headers: { "Idempotency-Key": t.key },
            });
        const receipt = requireSubjectReceipt(result, kind, t.id);
        t.id = receipt.entry_id;
        t.revision = receipt.revision;
        t.metadataDone = true;
        setPartial(true);
      }
      let s = requireSubject(
        await getBibleEntry({ pid: projectID, kind, id: t.id! }),
        projectID,
        kind,
        t.id,
      );
      if (kind === "character" && image && image !== subjectImage(s)) {
        if (!t.referenceBody) {
          if (s.head.revision !== t.revision)
            throw new AuthError("revision_conflict", 409);
          const look = s.current.character?.looks?.find((l) => l.default);
          if (!isProjectID(look?.id)) throw new AuthError("invalid_receipt");
          t.look = look.id;
          t.referenceBody = {
            expected_revision: s.head.revision,
            references: [
              ...(look.references ?? [])
                .filter((r) => r.role !== "primary")
                .map((r) => ({ role: r.role, asset_id: r.media?.asset_id })),
              { role: "primary", asset_id: image },
            ],
          };
        }
        const referenceReceipt = requireSubjectReceipt(
          await replaceBibleReferences(
            { pid: projectID, kind, id: t.id!, look: t.look! },
            t.referenceBody,
            { headers: { "Idempotency-Key": t.referenceKey } },
          ),
          kind,
          t.id,
        );
        t.revision = referenceReceipt.revision;
        s = requireSubject(
          await getBibleEntry({ pid: projectID, kind, id: t.id! }),
          projectID,
          kind,
          t.id,
        );
      }
      if (
        s.head.revision! < t.revision! ||
        (kind === "character" && image !== subjectImage(s)) ||
        subjectText(s).name !== name.trim() ||
        subjectText(s).description !== description.trim()
      )
        throw new AuthError("invalid_receipt");
      onSaved(s);
    } catch (e) {
      setError(flowError(e));
      if (
        !t.metadataDone &&
        e instanceof AuthError &&
        e.status >= 400 &&
        e.status < 500
      ) {
        transaction.current = null;
        setFrozen(false);
      }
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(v) => {
          if (!v) close();
        }}
      >
        <DialogContent
          className="subject-editor sm:max-w-[560px]"
          onInteractOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => {
            e.preventDefault();
            close();
          }}
        >
          <DialogTitle>
            {initial ? "编辑" : "创建"}
            {kinds[kind]}主体
          </DialogTitle>
          <DialogDescription className="sr-only">
            {kind === "character"
              ? "上传主参考图，保持角色在创作中的一致性。"
              : "用名称和描述定义项目中的" + kinds[kind] + "。"}
          </DialogDescription>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <fieldset disabled={pending || frozen}>
              {kind === "character" && (
                <label className="form-label">
                  参考主体 <span>*</span>
                  <button
                    type="button"
                    className="subject-upload"
                    aria-label="选择主体参考图"
                    onClick={() => setPicker(true)}
                  >
                    {image ? (
                      <ProjectMediaPreview
                        projectID={projectID}
                        id={image}
                        large
                      />
                    ) : (
                      <>
                        <ImagePlus size={32} />
                        <strong>添加主参考图</strong>
                        <span>从项目素材选择，或上传本地图片</span>
                      </>
                    )}
                  </button>
                </label>
              )}
              <label className="form-label">
                主体名称 <span>*</span>
                <Input
                  autoFocus
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                  maxLength={20}
                  placeholder="为主体起个名字"
                  aria-label="主体名称"
                />
                <small>{name.length}/20</small>
              </label>
              <label className="form-label">
                主体描述
                <Textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  maxLength={2000}
                  placeholder="描述外观、特征和创作中需要保持的细节"
                  aria-label="主体描述"
                />
                <small>{description.length}/2000</small>
              </label>
            </fieldset>
            {partial && error && (
              <p className="flow-notice">
                名称与描述已保存。参考图尚未确认，重试将继续完成同一个主体。
              </p>
            )}
            {error && (
              <p role="alert" className="flow-error">
                {error}
              </p>
            )}
            <div className="flow-actions">
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={close}
              >
                取消
              </Button>
              <Button
                disabled={
                  pending || !name.trim() || (kind === "character" && !image)
                }
              >
                {pending ? "正在保存…" : frozen ? "重试原保存" : "保存主体"}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
      {picker && (
        <ProjectMediaPicker
          projectID={projectID}
          onClose={() => setPicker(false)}
          onSelect={(a) => {
            setImage(a.id);
            setPicker(false);
          }}
        />
      )}
      {leave && (
        <Dialog open onOpenChange={setLeave}>
          <DialogContent role="alertdialog">
            <DialogTitle>离开主体编辑？</DialogTitle>
            <DialogDescription>
              {frozen
                ? "部分请求可能已保存。离开后请刷新主体列表核对实际结果。"
                : "未保存的输入会丢失。"}
            </DialogDescription>
            <div className="flow-actions">
              <Button variant="outline" onClick={() => setLeave(false)}>
                继续编辑
              </Button>
              <Button variant="destructive" onClick={onClose}>
                确认离开
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

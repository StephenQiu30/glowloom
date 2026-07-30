"use client";

import { CalendarRange, FileCheck2, ShieldCheck, X } from "lucide-react";
import { type FormEvent, useState } from "react";
import { Dialog } from "radix-ui";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export type ConsentFormValue = {
  subjectIdentity: API.SubjectIdentity;
  scope: API.MediaUsageScope;
  proofMediaVersionIds: string[];
  reason: string;
};

type ConsentFormDialogProps = {
  initialConsent?: API.ConsentDetailResponse;
  initialProofMediaVersionId?: string;
  initialSubjectId?: string;
  initialSubjectType?: API.SubjectType;
  isSubmitting: boolean;
  mediaVersions: API.MediaVersionResponse[];
  mode: "create" | "revise";
  onDirty?: () => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: (value: ConsentFormValue) => Promise<void>;
  open: boolean;
};

const rightsTypes = [
  { value: "copyright", label: "著作权" },
  { value: "image", label: "形象" },
  { value: "voice", label: "声音" },
] as const;

const purposes = [
  { value: "ai_short_drama_generation", label: "AI 漫剧生成" },
  { value: "public_distribution", label: "公开分发" },
  { value: "internal_demo", label: "内部演示" },
] as const;

const channels = [
  { value: "lanverse_preview", label: "平台预览" },
  { value: "lanverse_download", label: "受控下载" },
  { value: "public_export", label: "公开导出" },
] as const;

function dateInputValue(value?: string): string {
  if (value) return value.slice(0, 10);
  return new Date().toISOString().slice(0, 10);
}

function nextYearDate(value?: string): string {
  if (value) return value.slice(0, 10);
  const date = new Date();
  date.setUTCFullYear(date.getUTCFullYear() + 1);
  return date.toISOString().slice(0, 10);
}

function mediaLabel(media: API.MediaVersionResponse): string {
  return `${media.filename} · v${media.version_no} · ${media.probe_status}`;
}

function checkedValues(form: FormData, name: string): string[] {
  return form.getAll(name).map(String);
}

function CheckboxGroup({
  initialValues,
  items,
  legend,
  name,
}: {
  initialValues: string[];
  items: readonly { value: string; label: string }[];
  legend: string;
  name: string;
}) {
  return (
    <fieldset className="grid gap-2">
      <legend className="text-sm font-medium">{legend}</legend>
      <div className="flex flex-wrap gap-2">
        {items.map((item) => (
          <label
            className="flex cursor-pointer items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600 has-checked:border-cyan-300 has-checked:bg-cyan-50 has-checked:text-[#087f91]"
            key={item.value}
          >
            <input
              className="size-4 accent-[#079db3]"
              defaultChecked={initialValues.includes(item.value)}
              name={name}
              type="checkbox"
              value={item.value}
            />
            {item.label}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

export function ConsentFormDialog({
  initialConsent,
  initialProofMediaVersionId,
  initialSubjectId,
  initialSubjectType,
  isSubmitting,
  mediaVersions,
  mode,
  onDirty,
  onOpenChange,
  onSubmit,
  open,
}: ConsentFormDialogProps) {
  const current = initialConsent?.current_revision;
  const initialScope = current?.scope;
  const [subjectType, setSubjectType] = useState<API.SubjectType>(
    initialScope?.subject_type ?? initialSubjectType ?? "MEDIA_VERSION",
  );
  const effectiveInitialProofId =
    current?.proof_media_version_ids[0] ??
    initialProofMediaVersionId ??
    mediaVersions[0]?.id ??
    "";
  const effectiveInitialSubjectId =
    initialScope?.subject_id ?? initialSubjectId ?? mediaVersions[0]?.id ?? "";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const subjectType = String(form.get("subjectType")) as API.SubjectType;
    const subjectId = String(
      subjectType === "MEDIA_VERSION"
        ? form.get("mediaSubjectId")
        : subjectType === "ASSET_VERSION"
          ? form.get("assetSubjectId")
          : form.get("scriptSubjectId"),
    );
    await onSubmit({
      subjectIdentity: {
        reference: String(form.get("subjectReference")),
        kind: String(form.get("subjectKind")) as API.SubjectIdentityKind,
      },
      scope: {
        type: "media_usage",
        subject_type: subjectType,
        subject_id: subjectId,
        rights_holder_role: String(form.get("rightsHolderRole")),
        rights_types: checkedValues(form, "rightsTypes"),
        authorized_purposes: checkedValues(form, "purposes"),
        channels: checkedValues(form, "channels"),
        regions: ["CN"],
        valid_from: `${String(form.get("validFrom"))}T00:00:00.000Z`,
        valid_to: `${String(form.get("validTo"))}T23:59:59.999Z`,
      },
      proofMediaVersionIds: [String(form.get("proofMediaVersionId"))],
      reason: String(form.get("reason")),
    });
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-50 max-h-[calc(100vh-2rem)] w-[calc(100%-2rem)] max-w-3xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl border border-slate-200 bg-[#fbfcfd] shadow-2xl shadow-slate-950/15">
          <div className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-slate-200 bg-white px-6 py-5">
            <div>
              <Dialog.Title className="text-xl font-semibold tracking-tight">
                {mode === "create" ? "登记新授权" : "追加授权修订"}
              </Dialog.Title>
              <Dialog.Description className="mt-1 text-sm leading-6 text-slate-500">
                固定版本、范围与证明会作为不可变 revision 保存。
              </Dialog.Description>
            </div>
            <Dialog.Close asChild>
              <Button aria-label="关闭" size="icon" variant="ghost">
                <X aria-hidden="true" />
              </Button>
            </Dialog.Close>
          </div>

          <form className="grid gap-6 p-6" onChange={onDirty} onSubmit={submit}>
            <section className="grid gap-4 rounded-2xl border border-slate-200 bg-white p-5">
              <div className="flex items-center gap-2">
                <ShieldCheck className="size-4 text-[#079db3]" aria-hidden="true" />
                <h2 className="font-semibold">权利主体与固定版本</h2>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-subjectReference`}>权利主体引用</Label>
                  <Input
                    defaultValue={
                      initialConsent?.subject_identity.reference ??
                      "synthetic-subject-adult-a"
                    }
                    disabled={mode === "revise"}
                    id={`${mode}-subjectReference`}
                    name="subjectReference"
                    pattern="[A-Za-z0-9._:-]+"
                    required
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-subjectKind`}>主体类型</Label>
                  <select
                    className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm outline-none focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10 disabled:bg-slate-50"
                    defaultValue={
                      initialConsent?.subject_identity.kind ?? "fictional_adult"
                    }
                    disabled={mode === "revise"}
                    id={`${mode}-subjectKind`}
                    name="subjectKind"
                  >
                    <option value="fictional_adult">虚构成年角色</option>
                    <option value="adult">成年自然人</option>
                    <option value="organization">组织</option>
                    <option value="minor">未成年人（默认阻断）</option>
                  </select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-subjectType`}>版本类型</Label>
                  <select
                    className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm outline-none focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10"
                    id={`${mode}-subjectType`}
                    name="subjectType"
                    onChange={(event) => setSubjectType(event.target.value as API.SubjectType)}
                    value={subjectType}
                  >
                    <option value="MEDIA_VERSION">媒体版本</option>
                    <option value="SCRIPT_VERSION">剧本版本</option>
                    <option value="ASSET_VERSION">资产版本</option>
                  </select>
                </div>
                <div className="grid gap-2">
                  {subjectType === "MEDIA_VERSION" ? (
                    <>
                      <Label htmlFor={`${mode}-mediaSubjectId`}>固定版本</Label>
                      <select
                        className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm outline-none focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10"
                        defaultValue={effectiveInitialSubjectId}
                        id={`${mode}-mediaSubjectId`}
                        key={`media-subject-${effectiveInitialSubjectId}-${mediaVersions.length}`}
                        name="mediaSubjectId"
                        required
                      >
                        {mediaVersions.map((media) => (
                          <option key={media.id} value={media.id}>
                            {mediaLabel(media)}
                          </option>
                        ))}
                      </select>
                    </>
                  ) : subjectType === "ASSET_VERSION" ? (
                    <>
                      <Label htmlFor={`${mode}-assetSubjectId`}>资产版本 UUID</Label>
                      <Input
                        defaultValue={
                          initialScope?.subject_type === "ASSET_VERSION"
                            ? effectiveInitialSubjectId
                            : initialSubjectId ?? ""
                        }
                        id={`${mode}-assetSubjectId`}
                        name="assetSubjectId"
                        placeholder="输入 AssetVersion UUID"
                        required
                      />
                    </>
                  ) : (
                    <>
                      <Label htmlFor={`${mode}-scriptSubjectId`}>固定版本</Label>
                      <Input
                        defaultValue={
                          initialScope?.subject_type === "SCRIPT_VERSION"
                            ? effectiveInitialSubjectId
                            : ""
                        }
                        id={`${mode}-scriptSubjectId`}
                        name="scriptSubjectId"
                        placeholder="输入 ScriptVersion UUID"
                        required
                      />
                    </>
                  )}
                </div>
              </div>
            </section>

            <section className="grid gap-5 rounded-2xl border border-slate-200 bg-white p-5">
              <div className="flex items-center gap-2">
                <CalendarRange className="size-4 text-[#079db3]" aria-hidden="true" />
                <h2 className="font-semibold">使用范围与有效期</h2>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-rightsHolderRole`}>权利持有人角色</Label>
                  <Input
                    defaultValue={initialScope?.rights_holder_role ?? "synthetic_creator"}
                    id={`${mode}-rightsHolderRole`}
                    name="rightsHolderRole"
                    required
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-region`}>适用地域</Label>
                  <select
                    className="h-9 rounded-lg border border-slate-200 bg-slate-50 px-3 text-sm"
                    disabled
                    id={`${mode}-region`}
                    value="CN"
                  >
                    <option value="CN">中国大陆（CN）</option>
                  </select>
                </div>
              </div>
              <CheckboxGroup
                initialValues={initialScope?.rights_types ?? ["copyright", "image", "voice"]}
                items={rightsTypes}
                legend="授权权利"
                name="rightsTypes"
              />
              <CheckboxGroup
                initialValues={
                  initialScope?.authorized_purposes ?? [
                    "ai_short_drama_generation",
                    "public_distribution",
                  ]
                }
                items={purposes}
                legend="授权用途"
                name="purposes"
              />
              <CheckboxGroup
                initialValues={
                  initialScope?.channels ?? [
                    "lanverse_preview",
                    "lanverse_download",
                  ]
                }
                items={channels}
                legend="使用渠道"
                name="channels"
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-validFrom`}>生效日期</Label>
                  <Input
                    defaultValue={dateInputValue(initialScope?.valid_from)}
                    id={`${mode}-validFrom`}
                    name="validFrom"
                    required
                    type="date"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-validTo`}>到期日期</Label>
                  <Input
                    defaultValue={nextYearDate(initialScope?.valid_to)}
                    id={`${mode}-validTo`}
                    name="validTo"
                    required
                    type="date"
                  />
                </div>
              </div>
            </section>

            <section className="grid gap-4 rounded-2xl border border-slate-200 bg-white p-5">
              <div className="flex items-center gap-2">
                <FileCheck2 className="size-4 text-[#079db3]" aria-hidden="true" />
                <h2 className="font-semibold">证明与说明</h2>
              </div>
              <div className="grid gap-2">
                <Label htmlFor={`${mode}-proofMediaVersionId`}>证明媒体</Label>
                <select
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm outline-none focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10"
                  defaultValue={effectiveInitialProofId}
                  id={`${mode}-proofMediaVersionId`}
                  key={`proof-${effectiveInitialProofId}-${mediaVersions.length}`}
                  name="proofMediaVersionId"
                  required
                >
                  {mediaVersions.map((media) => (
                    <option key={media.id} value={media.id}>
                      {mediaLabel(media)}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor={`${mode}-reason`}>
                  {mode === "create" ? "登记说明" : "修订说明"}
                </Label>
                <textarea
                  className="min-h-24 resize-none rounded-xl border border-slate-200 bg-white px-3 py-3 text-sm leading-6 outline-none focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10"
                  defaultValue={mode === "revise" ? "调整授权使用范围" : "角色形象与声音授权"}
                  id={`${mode}-reason`}
                  maxLength={1000}
                  name="reason"
                  required
                />
              </div>
              <Alert className="border-cyan-100 bg-cyan-50/70 px-4 py-3 text-[#087f91]">
                <ShieldCheck aria-hidden="true" />
                <AlertTitle>最小必要展示</AlertTitle>
                <AlertDescription className="text-[#087f91]/80">
                  页面只保存 MediaVersion 引用，不展示或复制证明原文与存储地址。
                </AlertDescription>
              </Alert>
            </section>

            <div className="sticky bottom-0 -mx-6 -mb-6 flex justify-end gap-2 border-t border-slate-200 bg-white px-6 py-4">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">取消</Button>
              </Dialog.Close>
              <Button
                className="bg-[#079db3] px-5 text-white hover:bg-[#078da0]"
                disabled={isSubmitting || mediaVersions.length === 0}
                type="submit"
              >
                <ShieldCheck aria-hidden="true" />
                {isSubmitting
                  ? "保存中…"
                  : mode === "create"
                    ? "登记授权"
                    : "保存新修订"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

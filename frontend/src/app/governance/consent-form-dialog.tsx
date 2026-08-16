"use client";

import { CalendarRange, FileCheck2, ShieldCheck } from "lucide-react";
import { type FormEvent, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

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
            className="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted/70 has-checked:bg-muted has-checked:text-foreground"
            key={item.value}
          >
            <Checkbox
              defaultChecked={initialValues.includes(item.value)}
              name={name}
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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100vh-2rem)] max-w-3xl overflow-y-auto p-0">
        <DialogHeader className="sticky top-0 z-10 border-b bg-background px-6 py-5">
          <DialogTitle>
            {mode === "create" ? "登记新授权" : "追加授权修订"}
          </DialogTitle>
          <DialogDescription>
            固定版本、范围与证明会作为不可变 revision 保存。
          </DialogDescription>
        </DialogHeader>

          <form className="grid gap-6 p-6" onChange={onDirty} onSubmit={submit}>
            <section className="grid gap-4 bg-muted/45 p-5">
              <div className="flex items-center gap-2">
                <ShieldCheck className="size-4 text-foreground" aria-hidden="true" />
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
                  <Select
                    defaultValue={
                      initialConsent?.subject_identity.kind ?? "fictional_adult"
                    }
                    disabled={mode === "revise"}
                    name="subjectKind"
                  >
                    <SelectTrigger className="w-full" id={`${mode}-subjectKind`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="fictional_adult">虚构成年角色</SelectItem>
                      <SelectItem value="adult">成年自然人</SelectItem>
                      <SelectItem value="organization">组织</SelectItem>
                      <SelectItem value="minor">未成年人（默认阻断）</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor={`${mode}-subjectType`}>版本类型</Label>
                  <Select
                    name="subjectType"
                    onValueChange={(value) => setSubjectType(value as API.SubjectType)}
                    value={subjectType}
                  >
                    <SelectTrigger className="w-full" id={`${mode}-subjectType`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="MEDIA_VERSION">媒体版本</SelectItem>
                      <SelectItem value="SCRIPT_VERSION">剧本版本</SelectItem>
                      <SelectItem value="ASSET_VERSION">资产版本</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-2">
                  {subjectType === "MEDIA_VERSION" ? (
                    <>
                      <Label htmlFor={`${mode}-mediaSubjectId`}>固定版本</Label>
                      <Select
                        defaultValue={effectiveInitialSubjectId}
                        key={`media-subject-${effectiveInitialSubjectId}-${mediaVersions.length}`}
                        name="mediaSubjectId"
                        required
                      >
                        <SelectTrigger className="w-full" id={`${mode}-mediaSubjectId`}>
                          <SelectValue placeholder="选择固定版本" />
                        </SelectTrigger>
                        <SelectContent>
                          {mediaVersions.map((media) => (
                            <SelectItem key={media.id} value={media.id}>
                              {mediaLabel(media)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
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

            <section className="grid gap-5 bg-muted/45 p-5">
              <div className="flex items-center gap-2">
                <CalendarRange className="size-4 text-foreground" aria-hidden="true" />
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
                  <Select defaultValue="CN" disabled>
                    <SelectTrigger className="w-full" id={`${mode}-region`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="CN">中国大陆（CN）</SelectItem>
                    </SelectContent>
                  </Select>
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

            <section className="grid gap-4 bg-muted/45 p-5">
              <div className="flex items-center gap-2">
                <FileCheck2 className="size-4 text-foreground" aria-hidden="true" />
                <h2 className="font-semibold">证明与说明</h2>
              </div>
              <div className="grid gap-2">
                <Label htmlFor={`${mode}-proofMediaVersionId`}>证明媒体</Label>
                <Select
                  defaultValue={effectiveInitialProofId}
                  key={`proof-${effectiveInitialProofId}-${mediaVersions.length}`}
                  name="proofMediaVersionId"
                  required
                >
                  <SelectTrigger className="w-full" id={`${mode}-proofMediaVersionId`}>
                    <SelectValue placeholder="选择证明媒体" />
                  </SelectTrigger>
                  <SelectContent>
                    {mediaVersions.map((media) => (
                      <SelectItem key={media.id} value={media.id}>
                        {mediaLabel(media)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor={`${mode}-reason`}>
                  {mode === "create" ? "登记说明" : "修订说明"}
                </Label>
                <Textarea
                  className="min-h-24 resize-none leading-6"
                  defaultValue={mode === "revise" ? "调整授权使用范围" : "角色形象与声音授权"}
                  id={`${mode}-reason`}
                  maxLength={1000}
                  name="reason"
                  required
                />
              </div>
              <Alert className="border-border bg-muted/70 px-4 py-3 text-foreground">
                <ShieldCheck aria-hidden="true" />
                <AlertTitle>最小必要展示</AlertTitle>
                <AlertDescription className="text-foreground/80">
                  页面只保存 MediaVersion 引用，不展示或复制证明原文与存储地址。
                </AlertDescription>
              </Alert>
            </section>

            <DialogFooter className="sticky bottom-0 -mx-6 -mb-6 border-t bg-background px-6 py-4">
              <DialogClose asChild>
                <Button type="button" variant="outline">取消</Button>
              </DialogClose>
              <Button
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
            </DialogFooter>
          </form>
      </DialogContent>
    </Dialog>
  );
}

"use client";

import { X } from "lucide-react";
import { type FormEvent } from "react";
import { Dialog } from "radix-ui";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import {
  type AssetKind,
  assetTypes,
  buildSpec,
  dialogClassName,
  selectClassName,
  splitValues,
  textareaClassName,
  textValue,
  typeConfig,
} from "./asset-workspace-model";

function DialogHeading({
  description,
  title,
}: {
  description: string;
  title: string;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <Dialog.Title className="text-xl font-semibold tracking-tight">
          {title}
        </Dialog.Title>
        <Dialog.Description className="mt-1 text-sm leading-6 text-slate-500">
          {description}
        </Dialog.Description>
      </div>
      <Dialog.Close asChild>
        <Button aria-label="关闭" size="icon" variant="ghost">
          <X aria-hidden="true" />
        </Button>
      </Dialog.Close>
    </div>
  );
}

function assetDeleteBlockerMessage(blocker: API.AssetDeleteBlocker): string {
  if (blocker.code === "asset_has_versions" && blocker.version_count) {
    return `资产包含 ${blocker.version_count} 个不可变版本，不能删除。`;
  }
  if (
    blocker.code === "asset_has_candidate_decisions" &&
    blocker.decision_count
  ) {
    return `资产已被 ${blocker.decision_count} 条剧本候选决议关联，只能归档。`;
  }
  if (
    blocker.code === "asset_has_related_versions" &&
    blocker.related_version_count
  ) {
    return `资产已被 ${blocker.related_version_count} 个道具或服装版本引用，只能归档。`;
  }
  return blocker.summary;
}

export function CreateAssetDialog({
  currentKind,
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
}: {
  currentKind: AssetKind;
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.AssetCreateRequest) => Promise<boolean>;
  open: boolean;
}) {
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const completed = await onSubmit({
      kind: textValue(form, "kind") as AssetKind,
      name: textValue(form, "name"),
      aliases: splitValues(form.get("aliases")),
      tags: splitValues(form.get("tags")),
    });
    if (completed) formElement.reset();
  }

  return (
    <Dialog.Root onOpenChange={onOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className={dialogClassName}>
          <DialogHeading
            description="资产身份用于稳定引用，后续描述、参考媒体和授权都通过不可变版本追踪。"
            title="新建资产身份"
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2 sm:grid-cols-2 sm:gap-4">
              <div className="grid gap-2">
                <Label htmlFor="assetKind">资产类型</Label>
                <select
                  className={selectClassName}
                  defaultValue={currentKind}
                  id="assetKind"
                  name="kind"
                >
                  {assetTypes.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.singular}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="assetName">资产名称</Label>
                <Input id="assetName" name="name" placeholder="例如：顾清禾" required />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="assetAliases">别名（逗号分隔）</Label>
              <Input id="assetAliases" name="aliases" placeholder="清禾，顾小姐" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="assetTags">标签（逗号分隔）</Label>
              <Input id="assetTags" name="tags" placeholder="主角，第一季" />
            </div>
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">
                  取消
                </Button>
              </Dialog.Close>
              <Button
                className="bg-primary text-white hover:bg-primary/85"
                disabled={isSubmitting}
                type="submit"
              >
                {isSubmitting ? "创建中…" : "创建资产"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function EditAssetDialog({
  asset,
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
}: {
  asset: API.AssetResponse;
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.AssetUpdateRequest) => Promise<boolean>;
  open: boolean;
}) {
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await onSubmit({
      expected_revision: asset.revision,
      name: textValue(form, "name"),
      aliases: splitValues(form.get("aliases")),
      tags: splitValues(form.get("tags")),
    });
  }

  return (
    <Dialog.Root onOpenChange={onOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className={dialogClassName}>
          <DialogHeading
            description="只修改资产身份信息；资产类型、不可变版本和既有镜头引用不会改变。"
            title="编辑资产身份"
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="assetName">资产名称</Label>
              <Input defaultValue={asset.name} id="assetName" name="name" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="assetAliases">别名（逗号分隔）</Label>
              <Input
                defaultValue={asset.aliases.join(", ")}
                id="assetAliases"
                name="aliases"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="assetTags">标签（逗号分隔）</Label>
              <Input
                defaultValue={asset.tags.join(", ")}
                id="assetTags"
                name="tags"
              />
            </div>
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">取消</Button>
              </Dialog.Close>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? "保存中…" : "保存身份信息"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function DeleteAssetDialog({
  asset,
  isDeleting,
  isLoading,
  onConfirm,
  onOpenChange,
  open,
  preflight,
}: {
  asset: API.AssetResponse;
  isDeleting: boolean;
  isLoading: boolean;
  onConfirm: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  preflight?: API.AssetDeletePreflightResponse;
}) {
  return (
    <Dialog.Root onOpenChange={onOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className={dialogClassName}>
          <DialogHeading
            description={`删除“${asset.name}”的资产身份；本操作只允许无版本、无引用的空资产。`}
            title="删除资产身份"
          />
          <div className="mt-6 grid gap-4">
            {isLoading ? (
              <p className="text-sm text-slate-500">正在检查删除阻塞项…</p>
            ) : preflight?.allowed ? (
              <p className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm leading-6 text-amber-800">
                删除后该空资产身份无法恢复；不会删除任何媒体、授权或下游版本。
              </p>
            ) : preflight ? (
              <div className="rounded-xl border border-slate-200 p-4">
                <p className="text-sm font-medium">当前不能删除</p>
                <ul className="mt-2 grid gap-1 text-sm text-slate-500">
                  {preflight.blockers.map((blocker) => (
                    <li key={blocker.code}>{assetDeleteBlockerMessage(blocker)}</li>
                  ))}
                </ul>
              </div>
            ) : null}
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">取消</Button>
              </Dialog.Close>
              {preflight?.allowed ? (
                <Button
                  disabled={isDeleting}
                  type="button"
                  variant="destructive"
                  onClick={() => void onConfirm()}
                >
                  {isDeleting ? "删除中…" : "确认删除空资产"}
                </Button>
              ) : null}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function TextField({
  id,
  label,
  name,
  placeholder,
}: {
  id: string;
  label: string;
  name: string;
  placeholder?: string;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} name={name} placeholder={placeholder} />
    </div>
  );
}

function TextAreaField({
  id,
  label,
  name,
  placeholder,
}: {
  id: string;
  label: string;
  name: string;
  placeholder?: string;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <textarea
        className={textareaClassName}
        id={id}
        name={name}
        placeholder={placeholder}
      />
    </div>
  );
}

function RelatedCharacterField({
  characters,
  kind,
}: {
  characters: API.AssetResponse[];
  kind: "prop" | "costume";
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor="relatedCharacter">
        {kind === "prop" ? "持有角色" : "穿着角色"}
      </Label>
      <select
        className={selectClassName}
        defaultValue=""
        id="relatedCharacter"
        name="relatedCharacter"
      >
        <option value="">暂不关联</option>
        {characters.map((character) => (
          <option key={character.id} value={character.id}>
            {character.name}
          </option>
        ))}
      </select>
    </div>
  );
}

function VersionSpecFields({
  characters,
  kind,
}: {
  characters: API.AssetResponse[];
  kind: AssetKind;
}) {
  switch (kind) {
    case "character":
      return (
        <>
          <TextField id="identity" label="身份定位" name="identity" />
          <TextAreaField id="appearance" label="外观描述" name="appearance" />
          <TextField id="ageImpression" label="年龄观感" name="ageImpression" />
          <TextField
            id="temperament"
            label="性格特征（逗号分隔）"
            name="temperament"
          />
        </>
      );
    case "location":
      return (
        <>
          <TextAreaField
            id="spatialDescription"
            label="空间描述"
            name="spatialDescription"
          />
          <TextField id="timeWeather" label="时间与天气" name="timeWeather" />
          <TextField
            id="visualElements"
            label="视觉元素（逗号分隔）"
            name="visualElements"
          />
          <TextAreaField id="lighting" label="光线描述" name="lighting" />
        </>
      );
    case "prop":
    case "costume":
      return (
        <>
          <TextAreaField id="appearance" label="外观描述" name="appearance" />
          <TextField id="material" label="材质" name="material" />
          <TextAreaField id="usageContext" label="使用场景" name="usageContext" />
          <RelatedCharacterField characters={characters} kind={kind} />
        </>
      );
    case "visual_style":
      return (
        <>
          <TextAreaField
            id="visualLanguage"
            label="视觉语言"
            name="visualLanguage"
          />
          <TextField id="palette" label="色彩体系" name="palette" />
          <TextAreaField
            id="lightingLanguage"
            label="光影语言"
            name="lightingLanguage"
          />
          <TextField
            id="negativeConstraints"
            label="负面约束（逗号分隔）"
            name="negativeConstraints"
          />
        </>
      );
    case "voice":
      return (
        <>
          <div className="grid gap-2">
            <Label htmlFor="sourceKind">声音来源</Label>
            <select
              className={selectClassName}
              defaultValue=""
              id="sourceKind"
              name="sourceKind"
            >
              <option value="">请选择</option>
              <option value="synthetic_recording">合成录音</option>
              <option value="human_recording">真人录音</option>
              <option value="voice_clone">声音克隆</option>
            </select>
          </div>
          <TextField id="language" label="语言" name="language" placeholder="zh-CN" />
          <TextField
            id="performanceTraits"
            label="表演特征（逗号分隔）"
            name="performanceTraits"
          />
          <TextField
            id="allowedUsage"
            label="允许用途（逗号分隔）"
            name="allowedUsage"
          />
        </>
      );
  }
}

export function VersionDialog({
  asset,
  characters,
  isSubmitting,
  mediaVersions,
  onOpenChange,
  onSubmit,
  open,
}: {
  asset: API.AssetResponse;
  characters: API.AssetResponse[];
  isSubmitting: boolean;
  mediaVersions: API.MediaVersionResponse[];
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.AssetVersionCreateRequest) => Promise<boolean>;
  open: boolean;
}) {
  const config = typeConfig(asset.kind);
  const compatibleMedia = mediaVersions.filter((item) =>
    item.mime_type.startsWith(`${config.mediaKind}/`),
  );

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const mediaVersionId = textValue(form, "mediaVersionId");
    const completed = await onSubmit({
      spec: buildSpec(asset.kind, form),
      prompt_description: textValue(form, "promptDescription"),
      media_references: mediaVersionId
        ? [
            {
              media_version_id: mediaVersionId,
              purpose: config.mediaPurpose,
              position: 1,
            },
          ]
        : [],
      source_type: "manual",
      source_id: null,
      expected_current_version_id: asset.current_version_id,
      set_as_current: true,
    });
    if (completed) formElement.reset();
  }

  return (
    <Dialog.Root onOpenChange={onOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className={dialogClassName}>
          <DialogHeading
            description={`为“${asset.name}”追加不可变版本；旧版本、来源和授权判断继续保留。`}
            title={`添加${config.singular}版本`}
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <VersionSpecFields characters={characters} kind={asset.kind} />
            <div className="grid gap-2">
              <Label htmlFor="mediaVersionId">参考媒体</Label>
              <select
                className={selectClassName}
                defaultValue=""
                id="mediaVersionId"
                name="mediaVersionId"
              >
                <option value="">暂不绑定，保存为草稿</option>
                {compatibleMedia.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.filename} · v{item.version_no} · {item.probe_status}
                  </option>
                ))}
              </select>
              <p className="text-xs text-slate-500">
                {compatibleMedia.length === 0
                  ? "当前工作区没有兼容的媒体版本，仍可先保存草稿。"
                  : config.mediaOptional
                    ? "风格参考可选；选择后会固定到该媒体版本。"
                    : "准备度要求绑定兼容媒体并完成探测。"}
              </p>
            </div>
            <TextAreaField
              id="promptDescription"
              label="提示词描述"
              name="promptDescription"
              placeholder="记录生成时必须保持的一致性要求"
            />
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">
                  取消
                </Button>
              </Dialog.Close>
              <Button
                className="bg-primary text-white hover:bg-primary/85"
                disabled={isSubmitting}
                type="submit"
              >
                {isSubmitting ? "保存中…" : "保存版本"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

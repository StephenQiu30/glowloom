"use client";

import { X } from "lucide-react";
import { type FormEvent, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog as DialogRoot,
  DialogClose,
  DialogContent,
  DialogDescription,
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

import {
  type AssetKind,
  assetTypes,
  buildSpec,
  dialogClassName,
  splitValues,
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
        <DialogTitle className="text-xl font-semibold tracking-tight">
          {title}
        </DialogTitle>
        <DialogDescription className="mt-1 text-sm leading-6 text-muted-foreground">
          {description}
        </DialogDescription>
      </div>
      <DialogClose asChild>
        <Button aria-label="关闭" size="icon" variant="ghost">
          <X aria-hidden="true" />
        </Button>
      </DialogClose>
    </div>
  );
}

function DialogFrame({
  children,
  onOpenChange,
  open,
}: {
  children: ReactNode;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}) {
  return (
    <DialogRoot onOpenChange={onOpenChange} open={open}>
      <DialogContent className={dialogClassName} showCloseButton={false}>
        {children}
      </DialogContent>
    </DialogRoot>
  );
}

function SelectField({
  defaultValue,
  id,
  label,
  name,
  options,
  placeholder,
}: {
  defaultValue?: string;
  id: string;
  label: string;
  name: string;
  options: Array<{ label: string; value: string }>;
  placeholder?: string;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Select defaultValue={defaultValue} name={name}>
        <SelectTrigger className="w-full" id={id}>
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
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
    <DialogFrame onOpenChange={onOpenChange} open={open}>
          <DialogHeading
            description="资产身份用于稳定引用，后续描述、参考媒体和授权都通过不可变版本追踪。"
            title="新建资产身份"
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2 sm:grid-cols-2 sm:gap-4">
              <SelectField
                defaultValue={currentKind}
                id="assetKind"
                label="资产类型"
                name="kind"
                options={assetTypes.map((item) => ({
                  label: item.singular,
                  value: item.id,
                }))}
              />
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
              <DialogClose asChild>
                <Button type="button" variant="outline">
                  取消
                </Button>
              </DialogClose>
              <Button
                className="bg-primary text-white hover:bg-primary/85"
                disabled={isSubmitting}
                type="submit"
              >
                {isSubmitting ? "创建中…" : "创建资产"}
              </Button>
            </div>
          </form>
    </DialogFrame>
  );
}

export function CreateStateDialog({
  asset,
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
}: {
  asset: API.AssetResponse;
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.AssetStateCreateRequest) => Promise<boolean>;
  open: boolean;
}) {
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const completed = await onSubmit({
      state_key: textValue(form, "stateKey"),
      label: textValue(form, "label"),
      description: textValue(form, "description"),
      expected_asset_revision: asset.revision,
      idempotency_key: `create-asset-state:${crypto.randomUUID()}`,
    });
    if (completed) formElement.reset();
  }

  return (
    <DialogFrame onOpenChange={onOpenChange} open={open}>
          <DialogHeading
            description={`为“${asset.name}”建立可独立版本化的剧情状态；状态键创建后保持稳定。`}
            title="新建剧情状态"
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2 sm:grid-cols-2 sm:gap-4">
              <div className="grid gap-2">
                <Label htmlFor="stateKey">状态键</Label>
                <Input
                  id="stateKey"
                  name="stateKey"
                  pattern="[a-z0-9][a-z0-9_]{0,79}"
                  placeholder="例如：injured"
                  required
                />
                <p className="text-xs text-slate-500">使用小写英文、数字或下划线。</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="stateLabel">显示名称</Label>
                <Input
                  id="stateLabel"
                  name="label"
                  placeholder="例如：受伤状态"
                  required
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="stateDescription">状态说明</Label>
              <Input
                id="stateDescription"
                name="description"
                placeholder="例如：第 3 集雨夜追逐后，左臂带伤"
              />
            </div>
            <div className="flex justify-end gap-2">
              <DialogClose asChild>
                <Button type="button" variant="outline">取消</Button>
              </DialogClose>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? "创建中…" : "创建状态"}
              </Button>
            </div>
          </form>
    </DialogFrame>
  );
}

export function EditStateDialog({
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
  state,
}: {
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.AssetStateUpdateRequest) => Promise<boolean>;
  open: boolean;
  state: API.AssetStateResponse;
}) {
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await onSubmit({
      expected_revision: state.revision,
      idempotency_key: `update-asset-state:${crypto.randomUUID()}`,
      label: textValue(form, "label"),
      description: textValue(form, "description"),
    });
  }

  return (
    <DialogFrame onOpenChange={onOpenChange} open={open}>
          <DialogHeading
            description={`状态键“${state.state_key}”保持不变，只更新可读名称和说明。`}
            title="编辑剧情状态"
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="editStateLabel">状态名称</Label>
              <Input
                defaultValue={state.label}
                id="editStateLabel"
                name="label"
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="editStateDescription">状态说明</Label>
              <Input
                defaultValue={state.description}
                id="editStateDescription"
                name="description"
              />
            </div>
            <div className="flex justify-end gap-2">
              <DialogClose asChild>
                <Button type="button" variant="outline">取消</Button>
              </DialogClose>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? "保存中…" : "保存状态"}
              </Button>
            </div>
          </form>
    </DialogFrame>
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
      aliases: splitValues(form.get("aliases")),
      tags: splitValues(form.get("tags")),
    });
  }

  return (
    <DialogFrame onOpenChange={onOpenChange} open={open}>
          <DialogHeading
            description="只修改别名和标签；重命名需要单独完成影响预检。"
            title="编辑资产身份"
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
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
              <DialogClose asChild>
                <Button type="button" variant="outline">取消</Button>
              </DialogClose>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? "保存中…" : "保存身份信息"}
              </Button>
            </div>
          </form>
    </DialogFrame>
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
    <DialogFrame onOpenChange={onOpenChange} open={open}>
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
              <DialogClose asChild>
                <Button type="button" variant="outline">取消</Button>
              </DialogClose>
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
    </DialogFrame>
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
      <Textarea
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
    <SelectField
      id="relatedCharacter"
      label={kind === "prop" ? "持有角色" : "穿着角色"}
      name="relatedCharacter"
      options={characters.map((character) => ({
        label: character.name,
        value: character.id,
      }))}
      placeholder="暂不关联"
    />
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
          <SelectField
            id="sourceKind"
            label="声音来源"
            name="sourceKind"
            options={[
              { label: "合成录音", value: "synthetic_recording" },
              { label: "真人录音", value: "human_recording" },
              { label: "声音克隆", value: "voice_clone" },
            ]}
            placeholder="请选择"
          />
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
  state,
  characters,
  isSubmitting,
  mediaVersions,
  onOpenChange,
  onSubmit,
  open,
}: {
  asset: API.AssetResponse;
  state: API.AssetStateResponse;
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
      expected_revision: state.revision,
      expected_current_version_id: state.current_version_id,
      set_as_current: true,
    });
    if (completed) formElement.reset();
  }

  return (
    <DialogFrame onOpenChange={onOpenChange} open={open}>
          <DialogHeading
            description={`为“${asset.name}”追加不可变版本；旧版本、来源和授权判断继续保留。`}
            title={`添加${config.singular}版本`}
          />
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <VersionSpecFields characters={characters} kind={asset.kind} />
            <div className="grid gap-2">
              <SelectField
                id="mediaVersionId"
                label="参考媒体"
                name="mediaVersionId"
                options={compatibleMedia.map((item) => ({
                  label: `${item.filename} · v${item.version_no} · ${item.probe_status}`,
                  value: item.id,
                }))}
                placeholder="暂不绑定，保存为草稿"
              />
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
              <DialogClose asChild>
                <Button type="button" variant="outline">
                  取消
                </Button>
              </DialogClose>
              <Button
                className="bg-primary text-white hover:bg-primary/85"
                disabled={isSubmitting}
                type="submit"
              >
                {isSubmitting ? "保存中…" : "保存版本"}
              </Button>
            </div>
          </form>
    </DialogFrame>
  );
}

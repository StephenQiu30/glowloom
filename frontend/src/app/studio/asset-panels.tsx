"use client";

import {
  AlertCircle,
  Archive,
  CheckCircle2,
  Clock3,
  FileImage,
  History,
  Layers3,
  Pencil,
  Plus,
  ShieldAlert,
  Trash2,
} from "lucide-react";
import Link from "next/link";

import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/class-names";
import { appApiErrorMessage } from "@/lib/server-state";

import { formatDate, shortId, typeConfig } from "./asset-workspace-model";
import { AssetVersionUsage } from "./asset-version-usage";

export function AssetList({
  assets,
  statesByAssetId,
  isLoading,
  onSelect,
  selectedId,
}: {
  assets: API.AssetResponse[];
  statesByAssetId: Map<string, API.AssetStateResponse[]>;
  isLoading: boolean;
  onSelect: (id: string) => void;
  selectedId?: string;
}) {
  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 text-sm text-slate-500">
          正在读取资产身份…
        </CardContent>
      </Card>
    );
  }
  if (assets.length === 0) {
    return (
      <Card>
        <CardContent className="grid min-h-48 place-items-center p-6 text-center">
          <div>
            <Layers3 className="mx-auto size-6 text-slate-300" aria-hidden="true" />
            <p className="mt-3 text-sm font-medium">当前筛选下没有资产</p>
            <p className="mt-1 text-xs text-slate-500">新建身份后再补充描述和参考媒体。</p>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle>资产身份</CardTitle>
        <CardDescription>{assets.length} 项符合当前筛选</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-2 p-2">
        {assets.map((asset) => {
          const config = typeConfig(asset.kind);
          const Icon = config.icon;
          const states = statesByAssetId.get(asset.id) ?? [];
          const versionedStates = states.filter(
            (state) => state.current_version_id,
          ).length;
          return (
            <button
              aria-label={`选择资产 ${asset.name}`}
              aria-pressed={selectedId === asset.id}
              className={cn(
                "grid w-full grid-cols-[40px_minmax(0,1fr)_auto] items-center gap-3 rounded-xl border p-3 text-left transition",
                selectedId === asset.id
                  ? "border-foreground/25 bg-muted/70 shadow-sm "
                  : "border-transparent hover:border-slate-200 hover:bg-slate-50",
              )}
              key={asset.id}
              onClick={() => onSelect(asset.id)}
              type="button"
            >
              <span className="grid size-10 place-items-center rounded-xl bg-slate-100 text-slate-500">
                <Icon className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">{asset.name}</span>
                <span className="mt-0.5 block truncate text-xs text-slate-500">
                  {asset.aliases.length > 0
                    ? asset.aliases.join(" · ")
                    : `${config.singular}身份`}
                </span>
              </span>
              <span className="text-right">
                <Badge
                  className={
                    asset.status === "active"
                      ? "border-emerald-100 bg-emerald-50 text-emerald-700"
                      : "border-slate-200 bg-slate-100 text-slate-500"
                  }
                  variant="outline"
                >
                  {asset.status === "active" ? "使用中" : "已归档"}
                </Badge>
                <span className="mt-1 block font-mono text-[10px] text-slate-400">
                  {asset.status === "archived"
                    ? "状态不参与生产"
                    : versionedStates > 0
                    ? `${versionedStates}/${states.length} 状态有版本`
                    : `${states.length} 状态 · 无版本`}
                </span>
              </span>
            </button>
          );
        })}
      </CardContent>
    </Card>
  );
}

export function ArchivedAssetCard({
  asset,
  isRestoring,
  onDelete,
  onEdit,
  onRestore,
}: {
  asset: API.AssetResponse;
  isRestoring: boolean;
  onDelete: () => void;
  onEdit: () => void;
  onRestore: () => void;
}) {
  const config = typeConfig(asset.kind);
  const Icon = config.icon;
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <span className="grid size-11 place-items-center rounded-xl bg-slate-100 text-slate-500">
              <Icon className="size-5" aria-hidden="true" />
            </span>
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <CardTitle className="text-xl">{asset.name}</CardTitle>
                <Badge variant="outline">{config.singular}</Badge>
                <Badge variant="secondary">已归档</Badge>
              </div>
              <CardDescription className="mt-1">
                Asset {shortId(asset.id)} · revision {asset.revision}
              </CardDescription>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button aria-label="编辑资产身份" onClick={onEdit} variant="outline">
              <Pencil aria-hidden="true" />编辑身份
            </Button>
            <Button disabled={isRestoring} onClick={onRestore} variant="outline">
              <Archive aria-hidden="true" />恢复
            </Button>
            <Button aria-label="删除资产身份" onClick={onDelete} variant="ghost">
              <Trash2 aria-hidden="true" />删除
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-5 text-sm leading-6 text-slate-500">
        归档资产不进入资产圣经、准备度与新镜头绑定；恢复后可继续维护原有剧情状态和不可变版本。
      </CardContent>
    </Card>
  );
}

export function AssetStateBar({
  assetStatus,
  onCreate,
  onSelect,
  selectedId,
  states,
}: {
  assetStatus: API.AssetResponse["status"];
  onCreate: () => void;
  onSelect: (id: string) => void;
  selectedId: string;
  states: API.AssetStateResponse[];
}) {
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <CardTitle>剧情状态</CardTitle>
            <CardDescription>每个状态独立维护当前生产版本。</CardDescription>
          </div>
          <Button
            disabled={assetStatus === "archived"}
            onClick={onCreate}
            size="sm"
            variant="outline"
          >
            <Plus aria-hidden="true" />新建状态
          </Button>
        </div>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2 p-4">
        {states.map((state) => (
          <Button
            aria-pressed={state.id === selectedId}
            key={state.id}
            onClick={() => onSelect(state.id)}
            size="sm"
            variant={state.id === selectedId ? "default" : "outline"}
          >
            {state.label}
            <span className="text-xs opacity-70">
              {state.current_version_id ? "有版本" : "待补版本"}
            </span>
          </Button>
        ))}
      </CardContent>
    </Card>
  );
}

function readinessStyle(status: API.AssetReadinessResponse["status"]): string {
  if (status === "ready") return "border-emerald-200 bg-emerald-50 text-emerald-700";
  if (status === "blocked") return "border-rose-200 bg-rose-50 text-rose-700";
  return "border-amber-200 bg-amber-50 text-amber-700";
}

const readinessBlockerLabels: Record<string, string> = {
  asset_archived: "资产已归档，不能用于新的生产任务",
  consent_expired: "授权已过期，当前用途不再被覆盖",
  consent_missing: "缺少覆盖当前用途的有效授权",
  consent_revoked: "授权已撤回，新的生成与交付已被阻止",
  media_failed: "参考媒体探测失败",
  media_pending: "参考媒体仍在探测中",
  media_quarantined: "参考媒体已被隔离",
  media_unavailable: "引用的媒体版本不可用",
  required_field_missing: "资产规格字段尚未填写完整",
  required_media_missing: "缺少该资产类型必需的参考媒体",
  rights_dependency_unavailable: "授权治理暂时无法完成判断",
};

function ReadinessPanel({
  error,
  isLoading,
  readiness,
}: {
  error: unknown;
  isLoading: boolean;
  readiness?: API.AssetReadinessResponse;
}) {
  if (isLoading) {
    return <p className="text-sm text-slate-500">正在实时检查媒体与授权…</p>;
  }
  if (error) {
    return (
      <Alert className="border-rose-200 bg-rose-50 text-rose-800" variant="destructive">
        <AlertCircle aria-hidden="true" />
        <AlertTitle>准备度暂时无法计算</AlertTitle>
        <AlertDescription>{appApiErrorMessage(error)}</AlertDescription>
      </Alert>
    );
  }
  if (!readiness) return <p className="text-sm text-slate-500">尚无可检查版本。</p>;

  const requiresConsent = readiness.blockers.some(
    (blocker) => blocker.next_action === "review_asset_consent",
  );
  const consentParams = new URLSearchParams({
    subjectType: "ASSET_VERSION",
    subjectId: readiness.dependency_snapshot.asset_version_id,
  });
  const proofMediaVersionId = readiness.dependency_snapshot.media_version_ids[0];
  if (proofMediaVersionId) {
    consentParams.set("proofMediaVersionId", proofMediaVersionId);
  }
  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Badge className={readinessStyle(readiness.status)} variant="outline">
          {readiness.status === "ready"
            ? "可用于生成"
            : readiness.status === "blocked"
              ? "已阻断"
              : "草稿未完整"}
        </Badge>
        <p className="text-xs text-slate-400">
          {formatDate(readiness.dependency_snapshot.evaluated_at)} 实时计算
        </p>
      </div>
      {readiness.blockers.length === 0 ? (
        <div className="mt-4 flex items-start gap-3 rounded-xl bg-emerald-50 p-4 text-emerald-800">
          <CheckCircle2 className="mt-0.5 size-4" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium">媒体、字段与授权范围均满足当前用途</p>
            <p className="mt-1 text-xs text-emerald-700/80">可进入漫剧生成与预览流程。</p>
          </div>
        </div>
      ) : (
        <ul className="mt-4 grid gap-2">
          {readiness.blockers.map((blocker, index) => (
            <li
              className="flex items-start gap-3 rounded-xl border border-slate-200 bg-slate-50/70 p-3"
              key={`${blocker.code}-${blocker.dependency_id ?? index}`}
            >
              <ShieldAlert className="mt-0.5 size-4 text-amber-600" aria-hidden="true" />
              <div>
                <p className="text-sm font-medium">
                  {readinessBlockerLabels[blocker.code] ?? blocker.summary}
                </p>
                <p className="mt-1 font-mono text-[11px] text-slate-400">
                  {blocker.field_path ?? blocker.code}
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
      {requiresConsent ? (
        <Button asChild className="mt-4" variant="outline">
          <Link href={`/governance?${consentParams.toString()}`}>前往授权治理</Link>
        </Button>
      ) : null}
    </div>
  );
}

function specEntries(spec: API.AssetVersionResponse["spec"]): [string, string][] {
  switch (spec.kind) {
    case "character":
      return [
        ["身份定位", spec.identity ?? "未填写"],
        ["年龄观感", spec.age_impression ?? "未填写"],
        ["外观描述", spec.appearance ?? "未填写"],
        ["性格特征", spec.temperament?.join("、") || "未填写"],
      ];
    case "location":
      return [
        ["空间描述", spec.spatial_description ?? "未填写"],
        ["时间天气", spec.time_weather ?? "未填写"],
        ["视觉元素", spec.visual_elements?.join("、") || "未填写"],
        ["光线", spec.lighting ?? "未填写"],
      ];
    case "prop":
      return [
        ["外观描述", spec.appearance ?? "未填写"],
        ["材质", spec.material ?? "未填写"],
        ["使用场景", spec.usage_context ?? "未填写"],
        ["持有角色", spec.holder_character_id ? shortId(spec.holder_character_id) : "未关联"],
      ];
    case "costume":
      return [
        ["外观描述", spec.appearance ?? "未填写"],
        ["材质", spec.material ?? "未填写"],
        ["使用场景", spec.usage_context ?? "未填写"],
        ["穿着角色", spec.wearer_character_id ? shortId(spec.wearer_character_id) : "未关联"],
      ];
    case "visual_style":
      return [
        ["视觉语言", spec.visual_language ?? "未填写"],
        ["色彩体系", spec.palette ?? "未填写"],
        ["光影语言", spec.lighting_language ?? "未填写"],
        ["负面约束", spec.negative_constraints?.join("、") || "未填写"],
      ];
    case "voice":
      return [
        ["声音来源", spec.source_kind ?? "未填写"],
        ["语言", spec.language ?? "未填写"],
        ["表演特征", spec.performance_traits?.join("、") || "未填写"],
        ["允许用途", spec.allowed_usage?.join("、") || "未填写"],
      ];
  }
}

function VersionDetail({
  mediaById,
  version,
}: {
  mediaById: Map<string, API.MediaVersionResponse>;
  version: API.AssetVersionResponse;
}) {
  return (
    <div className="grid gap-5">
      <dl className="grid gap-3 sm:grid-cols-2">
        {specEntries(version.spec).map(([label, value]) => (
          <div className="rounded-xl border border-slate-200 p-3" key={label}>
            <dt className="text-xs font-medium text-slate-400">{label}</dt>
            <dd className="mt-1 text-sm leading-6 text-slate-700">{value}</dd>
          </div>
        ))}
      </dl>
      <div>
        <p className="text-xs font-medium text-slate-400">提示词描述</p>
        <p className="mt-2 text-sm leading-6 text-slate-700">
          {version.prompt_description || "未填写"}
        </p>
      </div>
      <div>
        <p className="text-xs font-medium text-slate-400">固定媒体版本</p>
        {version.media_references.length === 0 ? (
          <p className="mt-2 text-sm text-slate-500">尚未绑定参考媒体。</p>
        ) : (
          <div className="mt-2 flex flex-wrap gap-2">
            {version.media_references.map((reference) => {
              const media = mediaById.get(reference.media_version_id);
              return (
                <Badge key={`${reference.purpose}-${reference.position}`} variant="outline">
                  <FileImage aria-hidden="true" />
                  {media?.filename ?? shortId(reference.media_version_id)} · {reference.purpose}
                </Badge>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function VersionHistory({
  assetStatus,
  currentVersionId,
  isChangingCurrent,
  onSetCurrent,
  versions,
}: {
  assetStatus: API.AssetResponse["status"];
  currentVersionId: string | null;
  isChangingCurrent: boolean;
  onSetCurrent: (version: API.AssetVersionResponse) => void;
  versions: API.AssetVersionResponse[];
}) {
  return (
    <div className="divide-y divide-slate-100">
      {versions.map((version) => (
        <div
          className="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 py-3"
          key={version.id}
        >
          <span className="mt-0.5 grid size-8 place-items-center rounded-full bg-muted text-foreground">
            <History className="size-4" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-medium">
              版本 v{version.version_no}
              {currentVersionId === version.id ? (
                <Badge className="ml-2" variant="outline">
                  当前
                </Badge>
              ) : null}
            </p>
            <p className="mt-1 truncate text-xs text-slate-500">
              {version.source_type === "script_extraction_candidate"
                ? "脚本候选交接"
                : "手动创建"} · {shortId(version.content_hash)}
            </p>
          </div>
          <div className="grid justify-items-end gap-2">
            <time className="text-xs text-slate-400" dateTime={version.created_at}>
              {formatDate(version.created_at)}
            </time>
            {currentVersionId !== version.id ? (
              <Button
                aria-label={`设为当前资产版本 v${version.version_no}`}
                disabled={assetStatus === "archived" || isChangingCurrent}
                size="sm"
                variant="outline"
                onClick={() => onSetCurrent(version)}
              >
                设为当前
              </Button>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  );
}

export function AssetDetail({
  asset,
  currentState,
  isArchiving,
  isChangingCurrent,
  mediaById,
  onAddVersion,
  onDelete,
  onEdit,
  onSetCurrent,
  onToggleArchive,
  onUpgradeCompleted,
  onUpgradeError,
  readiness,
  readinessError,
  readinessLoading,
  versions,
  versionsLoading,
}: {
  asset: API.AssetResponse;
  currentState: API.AssetStateResponse;
  isArchiving: boolean;
  isChangingCurrent: boolean;
  mediaById: Map<string, API.MediaVersionResponse>;
  onAddVersion: () => void;
  onDelete: () => void;
  onEdit: () => void;
  onSetCurrent: (version: API.AssetVersionResponse) => void;
  onToggleArchive: () => void;
  onUpgradeCompleted: (shotCount: number) => void;
  onUpgradeError: (message: string) => void;
  readiness?: API.AssetReadinessResponse;
  readinessError: unknown;
  readinessLoading: boolean;
  versions: API.AssetVersionResponse[];
  versionsLoading: boolean;
}) {
  const config = typeConfig(asset.kind);
  const Icon = config.icon;
  const currentVersion =
    versions.find((version) => version.id === currentState.current_version_id) ??
    versions[0];

  return (
    <div className="grid gap-5">
      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <span className="grid size-11 place-items-center rounded-xl bg-muted text-foreground">
                <Icon className="size-5" aria-hidden="true" />
              </span>
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <CardTitle className="text-xl">{asset.name}</CardTitle>
                  <Badge variant="outline">{config.singular}</Badge>
                  {asset.warnings?.includes("duplicate_name") ? (
                    <Badge
                      className="border-amber-200 bg-amber-50 text-amber-700"
                      variant="outline"
                    >
                      同名提醒
                    </Badge>
                  ) : null}
                </div>
                <CardDescription className="mt-1">
                  Asset {shortId(asset.id)} · {currentState.label} · state revision{" "}
                  {currentState.revision}
                </CardDescription>
                {asset.tags.length > 0 ? (
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {asset.tags.map((tag) => (
                      <Badge key={tag} variant="outline">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                ) : null}
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button aria-label="编辑资产身份" onClick={onEdit} variant="outline">
                <Pencil aria-hidden="true" />编辑身份
              </Button>
              <Button
                disabled={asset.status === "archived"}
                onClick={onAddVersion}
                variant="outline"
              >
                <Plus aria-hidden="true" />添加新版本
              </Button>
              <Button disabled={isArchiving} onClick={onToggleArchive} variant="ghost">
                <Archive aria-hidden="true" />
                {asset.status === "active" ? "归档" : "恢复"}
              </Button>
              <Button aria-label="删除资产身份" onClick={onDelete} variant="ghost">
                <Trash2 aria-hidden="true" />删除
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-5">
          {versionsLoading ? (
            <p className="text-sm text-slate-500">正在读取当前版本…</p>
          ) : currentVersion ? (
            <VersionDetail mediaById={mediaById} version={currentVersion} />
          ) : (
            <div className="grid min-h-36 place-items-center rounded-xl border border-dashed border-slate-200 text-center">
              <div>
                <Clock3 className="mx-auto size-5 text-slate-300" aria-hidden="true" />
                <p className="mt-2 text-sm font-medium">资产身份尚无版本</p>
                <p className="mt-1 text-xs text-slate-500">添加第一个版本后才能检查准备度。</p>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-4">
          <CardTitle>生产准备度</CardTitle>
          <CardDescription>基于当前版本的字段、媒体探测和实时授权事实计算。</CardDescription>
        </CardHeader>
        <CardContent className="p-5">
          <ReadinessPanel
            error={readinessError}
            isLoading={readinessLoading}
            readiness={readiness}
          />
        </CardContent>
      </Card>

      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-4">
          <CardTitle>版本历史</CardTitle>
          <CardDescription>版本内容不可覆盖，旧事实始终可追溯。</CardDescription>
        </CardHeader>
        <CardContent className="px-5 py-1">
          {versions.length > 0 ? (
            <VersionHistory
              assetStatus={asset.status}
              currentVersionId={currentState.current_version_id}
              isChangingCurrent={isChangingCurrent}
              versions={versions}
              onSetCurrent={onSetCurrent}
            />
          ) : (
            <p className="py-5 text-sm text-slate-500">暂无版本记录。</p>
          )}
        </CardContent>
      </Card>

      <AssetVersionUsage
        asset={asset}
        currentVersionId={currentState.current_version_id}
        onCompleted={onUpgradeCompleted}
        onError={onUpgradeError}
        versions={versions}
      />
    </div>
  );
}

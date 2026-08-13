"use client";

import {
  ArrowRightLeft,
  CheckCircle2,
  History,
  LoaderCircle,
  ShieldCheck,
} from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  appApiErrorMessage,
  useApplyAssetUpgradeMutation,
  useAssetShotUsagesQuery,
  useAssetUpgradePreflightMutation,
} from "@/lib/server-state";

import { selectClassName, shortId } from "./asset-workspace-model";

function versionLabel(
  version: API.AssetVersionResponse,
  currentVersionId: string | null,
): string {
  return `v${version.version_no}${version.id === currentVersionId ? "（资产当前版本）" : ""}`;
}

export function AssetVersionUsage({
  asset,
  currentVersionId,
  onCompleted,
  onError,
  versions,
}: {
  asset: API.AssetResponse;
  currentVersionId: string | null;
  onCompleted: (shotCount: number) => void;
  onError: (message: string) => void;
  versions: API.AssetVersionResponse[];
}) {
  const [sourceChoice, setSourceChoice] = useState<string | null>(null);
  const [targetChoice, setTargetChoice] = useState<string | null>(null);
  const [selection, setSelection] = useState<{
    sourceVersionId: string | null;
    shotIds: string[];
  }>({ sourceVersionId: null, shotIds: [] });
  const [usageOffset, setUsageOffset] = useState(0);
  const [preflight, setPreflight] =
    useState<API.AssetUpgradePreflightResponse | null>(null);
  const [reviewOpen, setReviewOpen] = useState(false);

  const sourceVersion =
    versions.find((version) => version.id === sourceChoice) ??
    versions.find((version) => version.id !== currentVersionId) ??
    versions.find((version) => version.id === currentVersionId) ??
    versions[0];
  const targetVersion =
    versions.find(
      (version) =>
        version.id === targetChoice && version.id !== sourceVersion?.id,
    ) ??
    versions.find(
      (version) =>
        version.id === currentVersionId &&
        version.id !== sourceVersion?.id,
    ) ??
    versions.find((version) => version.id !== sourceVersion?.id);
  const usageLimit = 20;
  const usages = useAssetShotUsagesQuery(
    {
      assetVersionId: sourceVersion?.id ?? "",
      limit: usageLimit,
      offset: usageOffset,
    },
    { skip: !sourceVersion },
  );
  const [runPreflight, preflightState] = useAssetUpgradePreflightMutation();
  const [applyUpgrade, applyState] = useApplyAssetUpgradeMutation();
  const usageItems = usages.data?.items ?? [];
  const currentUsages = usageItems.filter((usage) => usage.is_current);
  const historicalUsages = usageItems.filter((usage) => !usage.is_current);
  const selectedShotIds =
    selection.sourceVersionId === sourceVersion?.id ? selection.shotIds : [];
  const busy = preflightState.isLoading || applyState.isLoading;

  function resetReview() {
    setPreflight(null);
    setReviewOpen(false);
  }

  function changeSource(versionId: string) {
    setSourceChoice(versionId);
    setTargetChoice(null);
    setSelection({ sourceVersionId: null, shotIds: [] });
    setUsageOffset(0);
    resetReview();
  }

  function changeTarget(versionId: string) {
    setTargetChoice(versionId);
    resetReview();
  }

  function toggleShot(shotId: string) {
    setSelection((current) => {
      const currentShotIds =
        current.sourceVersionId === sourceVersion?.id ? current.shotIds : [];
      return {
        sourceVersionId: sourceVersion?.id ?? null,
        shotIds: currentShotIds.includes(shotId)
          ? currentShotIds.filter((candidate) => candidate !== shotId)
          : [...currentShotIds, shotId],
      };
    });
    resetReview();
  }

  async function prepareUpgrade() {
    if (!sourceVersion || !targetVersion || selectedShotIds.length === 0) return;
    onError("");
    try {
      const result = await runPreflight({
        assetVersionId: sourceVersion.id,
        body: {
          new_asset_version_id: targetVersion.id,
          shot_ids: selectedShotIds,
        },
      }).unwrap();
      setPreflight(result);
      setReviewOpen(true);
    } catch (error: unknown) {
      onError(appApiErrorMessage(error));
    }
  }

  async function confirmUpgrade() {
    if (!sourceVersion || !preflight) return;
    onError("");
    try {
      const result = await applyUpgrade({
        assetVersionId: sourceVersion.id,
        body: {
          new_asset_version_id: preflight.new_asset_version_id,
          targets: preflight.targets,
          preflight_hash: preflight.preflight_hash,
        },
      }).unwrap();
      setReviewOpen(false);
      setPreflight(null);
      setSelection({ sourceVersionId: null, shotIds: [] });
      onCompleted(result.shots.length);
    } catch (error: unknown) {
      setReviewOpen(false);
      setPreflight(null);
      onError(appApiErrorMessage(error));
    }
  }

  return (
    <>
      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-4">
          <CardTitle>
            <h2>分镜引用与版本升级</h2>
          </CardTitle>
          <CardDescription>
            查看不可变引用；升级只为选中的当前镜头追加新规格，历史版本不会被改写。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-5 p-5">
          {versions.length === 0 ? (
            <p className="text-sm text-slate-500">添加资产版本后才能检查引用。</p>
          ) : (
            <>
              <div className="grid gap-4 md:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor={`usage-source-${asset.id}`}>检查引用的资产版本</Label>
                  <select
                    className={selectClassName}
                    id={`usage-source-${asset.id}`}
                    onChange={(event) => changeSource(event.target.value)}
                    value={sourceVersion?.id ?? ""}
                  >
                    {versions.map((version) => (
                      <option key={version.id} value={version.id}>
                        {versionLabel(version, currentVersionId)}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor={`usage-target-${asset.id}`}>升级到资产版本</Label>
                  <select
                    className={selectClassName}
                    disabled={!targetVersion}
                    id={`usage-target-${asset.id}`}
                    onChange={(event) => changeTarget(event.target.value)}
                    value={targetVersion?.id ?? ""}
                  >
                    {versions
                      .filter((version) => version.id !== sourceVersion?.id)
                      .map((version) => (
                        <option key={version.id} value={version.id}>
                          {versionLabel(version, currentVersionId)}
                        </option>
                      ))}
                  </select>
                  {!targetVersion ? (
                    <p className="text-xs text-slate-500">添加另一个版本后才能升级。</p>
                  ) : null}
                </div>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex flex-wrap gap-2">
                  <Badge className="border-border bg-muted text-foreground" variant="outline">
                    本页当前引用 {currentUsages.length}
                  </Badge>
                  <Badge variant="outline">本页历史引用 {historicalUsages.length}</Badge>
                  <Badge variant="outline">引用总数 {usages.data?.total ?? 0}</Badge>
                </div>
                {currentUsages.length > 0 ? (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      setSelection({
                        sourceVersionId: sourceVersion?.id ?? null,
                        shotIds: currentUsages.every((usage) =>
                          selectedShotIds.includes(usage.shot_id),
                        )
                          ? selectedShotIds.filter(
                              (shotId) =>
                                !currentUsages.some(
                                  (usage) => usage.shot_id === shotId,
                                ),
                            )
                          : Array.from(
                              new Set([
                                ...selectedShotIds,
                                ...currentUsages.map((usage) => usage.shot_id),
                              ]),
                            ),
                      });
                      resetReview();
                    }}
                    size="sm"
                    variant="ghost"
                  >
                    {currentUsages.every((usage) =>
                      selectedShotIds.includes(usage.shot_id),
                    )
                      ? "取消全选"
                      : "全选当前引用"}
                  </Button>
                ) : null}
              </div>

              {usages.isLoading ? (
                <p className="flex items-center gap-2 text-sm text-slate-500">
                  <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                  正在读取分镜引用…
                </p>
              ) : usages.error ? (
                <Alert className="border-rose-200 bg-rose-50 text-rose-800" variant="destructive">
                  <AlertTitle>引用暂时无法读取</AlertTitle>
                  <AlertDescription>{appApiErrorMessage(usages.error)}</AlertDescription>
                </Alert>
              ) : usageItems.length === 0 ? (
                <div className="rounded-xl border border-dashed border-slate-200 p-5 text-center">
                  <CheckCircle2 className="mx-auto size-5 text-slate-300" aria-hidden="true" />
                  <p className="mt-2 text-sm font-medium">该版本尚未被分镜规格引用</p>
                  <p className="mt-1 text-xs text-slate-500">无需执行批量升级。</p>
                </div>
              ) : (
                <div className="max-h-72 divide-y divide-slate-100 overflow-y-auto rounded-xl border border-slate-200">
                  {currentUsages.map((usage) => (
                    <label
                      className="grid cursor-pointer grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3 p-3 hover:bg-slate-50"
                      key={usage.spec_version_id}
                    >
                      <input
                        aria-label={`选择镜头 ${usage.shot_title}`}
                        checked={selectedShotIds.includes(usage.shot_id)}
                        className="mt-1 size-4 accent-black"
                        disabled={busy || !targetVersion}
                        onChange={() => toggleShot(usage.shot_id)}
                        type="checkbox"
                      />
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-medium">
                          {usage.shot_title}
                        </span>
                        <span className="mt-1 block text-xs text-slate-500">
                          镜头规格 v{usage.spec_version_no} · 固定槽位 {usage.slot_keys.join("、")}
                        </span>
                      </span>
                      <Badge className="border-emerald-100 bg-emerald-50 text-emerald-700" variant="outline">
                        当前规格
                      </Badge>
                    </label>
                  ))}
                  {historicalUsages.map((usage) => (
                    <div
                      className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3 bg-slate-50/60 p-3"
                      key={usage.spec_version_id}
                    >
                      <History className="mt-0.5 size-4 text-slate-400" aria-hidden="true" />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-slate-600">
                          {usage.shot_title} · 规格 v{usage.spec_version_no}
                        </p>
                        <p className="mt-1 text-xs text-slate-500">
                          历史规格引用，不参与批量升级
                        </p>
                      </div>
                      <Badge variant="outline">历史规格</Badge>
                    </div>
                  ))}
                </div>
              )}

              {(usages.data?.total ?? 0) > usageLimit ? (
                <div className="flex items-center justify-between gap-3">
                  <p className="text-xs text-slate-500">
                    第 {Math.floor(usageOffset / usageLimit) + 1} 页 · 每页 {usageLimit} 项
                  </p>
                  <div className="flex gap-2">
                    <Button
                      disabled={usageOffset === 0 || usages.isFetching}
                      onClick={() => setUsageOffset((current) => Math.max(0, current - usageLimit))}
                      size="sm"
                      variant="outline"
                    >
                      上一页
                    </Button>
                    <Button
                      disabled={
                        usageOffset + usageLimit >= (usages.data?.total ?? 0) ||
                        usages.isFetching
                      }
                      onClick={() => setUsageOffset((current) => current + usageLimit)}
                      size="sm"
                      variant="outline"
                    >
                      下一页
                    </Button>
                  </div>
                </div>
              ) : null}

              <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-slate-50 p-4">
                <p className="text-sm text-slate-600">
                  已选择 <span className="font-semibold text-slate-950">{selectedShotIds.length}</span> 个当前镜头
                </p>
                <Button
                  disabled={!targetVersion || selectedShotIds.length === 0 || busy}
                  onClick={prepareUpgrade}
                >
                  {preflightState.isLoading ? (
                    <LoaderCircle className="animate-spin" aria-hidden="true" />
                  ) : (
                    <ArrowRightLeft aria-hidden="true" />
                  )}
                  生成升级预检
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={reviewOpen && Boolean(preflight)}
        onOpenChange={(open) => setReviewOpen(open)}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>确认资产版本升级</DialogTitle>
            <DialogDescription>
              {sourceVersion && targetVersion
                ? `${versionLabel(sourceVersion, currentVersionId)} → ${versionLabel(targetVersion, currentVersionId)}`
                : "复核即将写入的变更"}
            </DialogDescription>
          </DialogHeader>
          {preflight ? (
            <div className="grid gap-4">
              <Alert className="border-border bg-muted text-foreground">
                <ShieldCheck aria-hidden="true" />
                <AlertTitle>追加写入，不覆盖已有事实</AlertTitle>
                <AlertDescription>
                  <span className="block">旧规格和历史引用会继续保留</span>
                  系统将为 {preflight.targets.length} 个镜头各创建一个新规格版本，并把它设为当前版本。
                </AlertDescription>
              </Alert>
              <div className="max-h-56 divide-y divide-slate-100 overflow-y-auto rounded-xl border border-slate-200">
                {preflight.targets.map((target) => {
                  const usage = currentUsages.find(
                    (candidate) => candidate.shot_id === target.shot_id,
                  );
                  return (
                    <div className="flex items-center justify-between gap-3 p-3" key={target.shot_id}>
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">
                          {usage?.shot_title ?? `镜头 ${shortId(target.shot_id)}`}
                        </p>
                        <p className="mt-1 text-xs text-slate-500">
                          替换槽位 {target.slot_keys.join("、")}
                        </p>
                      </div>
                      <Badge variant="outline">新规格</Badge>
                    </div>
                  );
                })}
              </div>
              <p className="font-mono text-[11px] text-slate-400">
                预检 {shortId(preflight.preflight_hash)} · 提交前会再次校验当前规格与镜头 revision
              </p>
            </div>
          ) : null}
          <DialogFooter>
            <Button disabled={applyState.isLoading} onClick={() => setReviewOpen(false)} variant="outline">
              返回检查
            </Button>
            <Button disabled={!preflight || applyState.isLoading} onClick={confirmUpgrade}>
              {applyState.isLoading ? (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              ) : (
                <ArrowRightLeft aria-hidden="true" />
              )}
              应用升级并创建新规格版本
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

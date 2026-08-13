import { ArrowRight, CheckCircle2, CircleDashed, ShieldAlert } from "lucide-react";
import Link from "next/link";

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

import { assetKindLabels } from "./episode-studio-model";

export function EpisodeAssetOverview({
  summary,
  assets,
  assetBible,
}: {
  summary: API.AssetSummary;
  assets: API.AssetResponse[];
  assetBible?: API.AssetBibleResponse;
}) {
  const required = new Set(summary.required_kinds);
  const ready = new Set(summary.ready_kinds);
  const statesByAssetId = new Map(
    (assetBible?.items ?? []).map(({ asset, states }) => [asset.id, states]),
  );

  return (
    <div className="grid gap-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Card><CardHeader><CardDescription>资产身份</CardDescription><CardTitle className="text-3xl">{summary.total}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>已有版本</CardDescription><CardTitle className="text-3xl">{summary.versioned}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>可用于生成</CardDescription><CardTitle className="text-3xl text-emerald-700">{summary.ready}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>被阻断</CardDescription><CardTitle className="text-3xl text-rose-700">{summary.blocked}</CardTitle></CardHeader></Card>
      </div>

      <Card>
        <CardHeader className="gap-4 md:flex-row md:items-start md:justify-between">
          <div>
            <CardTitle>S2 必需资产</CardTitle>
            <CardDescription>角色、场景、声音各至少一项通过字段、媒体和授权准备度。</CardDescription>
          </div>
          <Button asChild><Link href="/studio">打开完整资产库<ArrowRight aria-hidden="true" /></Link></Button>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          {["character", "location", "voice"].map((kind) => {
            const isReady = ready.has(kind);
            return (
              <div className={`rounded-xl border p-4 ${isReady ? "border-emerald-200 bg-emerald-50" : "border-amber-200 bg-amber-50"}`} key={kind}>
                <div className="flex items-center gap-2">
                  {isReady ? <CheckCircle2 className="size-5 text-emerald-600" aria-hidden="true" /> : <CircleDashed className="size-5 text-amber-600" aria-hidden="true" />}
                  <h3 className="font-medium">{assetKindLabels[kind as API.AssetResponse["kind"]]}</h3>
                </div>
                <p className={`mt-2 text-sm ${isReady ? "text-emerald-700" : "text-amber-700"}`}>
                  {isReady ? "已有 ready 版本" : required.has(kind) ? "仍需补齐 ready 版本" : "非当前门禁"}
                </p>
              </div>
            );
          })}
        </CardContent>
      </Card>

      {summary.blocked ? (
        <Alert className="border-amber-200 bg-amber-50 text-amber-800">
          <ShieldAlert aria-hidden="true" />
          <AlertTitle>{summary.blocked} 个版本被阻断</AlertTitle>
          <AlertDescription className="text-amber-700">
            打开完整资产库查看具体字段、媒体探测或授权 blocker，并从治理工作台处理。
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>项目资产目录</CardTitle>
          <CardDescription>{assets.length} 项真实服务端资产身份</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {assets.map((asset) => (
            <article className="rounded-xl border border-slate-200 p-4" key={asset.id}>
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate font-medium">{asset.name}</p>
                  <p className="mt-1 text-xs text-slate-500">{assetKindLabels[asset.kind]} · revision {asset.revision}</p>
                </div>
                <Badge
                  variant={
                    statesByAssetId
                      .get(asset.id)
                      ?.some(({ state }) => state.current_version_id)
                      ? "secondary"
                      : "outline"
                  }
                >
                  {statesByAssetId
                    .get(asset.id)
                    ?.some(({ state }) => state.current_version_id)
                    ? "已有状态版本"
                    : "待建版本"}
                </Badge>
              </div>
            </article>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

"use client";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { appApiErrorMessage } from "@/lib/server-state";
import { AlertCircle, LoaderCircle } from "lucide-react";
import { Fact } from "./review-fact";
import { shortHash, shortId } from "./review-presentation";

export function StructureIdentityResultPanel({
  data,
  error,
  isFetching,
  verified,
}: {
  data: API.StructureIdentitySnapshotResponse | undefined;
  error: unknown;
  isFetching: boolean;
  verified: boolean;
}) {
  return (
    <Card aria-label="正式结构身份结果" className="border" id="structure-identity-result" role="region">
      <CardHeader className="border-b">
        <CardTitle>正式结构身份结果</CardTitle>
        <CardDescription>
          直接读取 Backend 已发布版本，并核对当前审核决议、Gate 输入和 Owner Receipt。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-5 pt-1">
        {error ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>正式结果无法回读</AlertTitle>
            <AlertDescription>{appApiErrorMessage(error)}</AlertDescription>
          </Alert>
        ) : !data ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            {isFetching ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin" /> : null}
            正在核对正式结构身份结果。
          </p>
        ) : !verified ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>正式结果与本次审核不一致</AlertTitle>
            <AlertDescription>已停止显示完成状态，请重新核对 Backend 当前版本。</AlertDescription>
          </Alert>
        ) : (
          <>
            <div className="grid gap-4 text-sm sm:grid-cols-2 xl:grid-cols-4">
              <Fact label="正式版本" value={`${data.version.version} · ${shortId(data.version.id)}`} mono />
              <Fact label="剧集 / 场景" value={`${data.version.episode_refs.length} / ${data.version.scene_refs.length}`} />
              <Fact label="身份" value={String(data.version.identities.length)} />
              <Fact
                label="提及覆盖"
                value={`${data.version.coverage.resolved_count} / ${data.version.coverage.mention_count}`}
              />
              <Fact label="未决提及" value={String(data.version.coverage.unresolved_count)} />
              <Fact label="Owner Receipt" value={shortId(data.command_receipt_id)} mono />
              <Fact label="Collection Root" value={shortHash(data.receipt.collection_root_hash)} mono />
              <Fact label="内容 Hash" value={shortHash(data.version.content_hash)} mono />
            </div>
            <div className="grid gap-4 lg:grid-cols-2">
              <section aria-label="正式剧集范围" className="border p-4">
                <h3 className="text-sm font-semibold">剧集范围</h3>
                <ul className="mt-3 grid gap-2 text-sm">
                  {data.version.episode_refs.map((episode) => (
                    <li className="flex justify-between gap-4" key={episode.episode_id}>
                      <span>第 {episode.position} 集</span>
                      <span className="font-mono text-xs text-muted-foreground">
                        [{episode.source_start}, {episode.source_end})
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
              <section aria-label="正式身份列表" className="border p-4">
                <h3 className="text-sm font-semibold">身份列表</h3>
                <ul className="mt-3 grid gap-2 text-sm">
                  {data.version.identities.map((identity) => (
                    <li key={identity.identity_key}>
                      <span className="font-medium">{identity.canonical_name}</span>
                      <span className="ml-2 text-xs text-muted-foreground">
                        {identity.kind} · {identity.aliases.join("、")}
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

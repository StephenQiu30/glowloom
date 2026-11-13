"use client";

import { useState } from "react";
import useSWR from "swr";
import { readAdminRoutePool, publishAdminRoutePool } from "@/api/settings";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { catalogMessage } from "@/components/catalog/mutation";

export function ModelRoutePool({ releaseID }: { releaseID: string }) {
  const [open, setOpen] = useState(false);
  const { data, mutate, error } = useSWR(
    open ? ["admin-route-pool", releaseID] : null,
    () => readAdminRoutePool({ id: releaseID }),
    { revalidateOnFocus: false },
  );
  const [draft, setDraft] = useState<{ base: number; ids: string[] } | null>(
    null,
  );
  const [review, setReview] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const ids = draft?.ids ?? data?.backups ?? [];
  const options = data?.options ?? [];
  function edit(next: string[]) {
    setDraft({ base: draft?.base ?? data?.revision ?? 0, ids: next });
    setReview(false);
    setMessage("");
  }
  async function publish() {
    if (!draft || busy) return;
    setBusy(true);
    try {
      const result = await publishAdminRoutePool(
        { id: releaseID },
        {
          expected_revision: draft.base,
          backups: draft.ids,
          equivalent: review,
          request_id: crypto.randomUUID(),
        },
      );
      await mutate(result, false);
      setDraft(null);
      setReview(false);
      setMessage("线路已发布");
    } catch (e) {
      setMessage(catalogMessage(e));
      await mutate();
      setDraft(null);
      setReview(false);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="space-y-2">
      <Button
        type="button"
        variant="outline"
        size="sm"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        备用线路
      </Button>
      {open && (
        <div className="space-y-2">
          {error && <p role="alert">{catalogMessage(error)}</p>}
          {data && (
            <>
              <p className="text-sm">当前版本 {data.revision} · 主线路优先</p>
              {options.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  暂无通过相同规格验证的备用线路。
                </p>
              )}
              {Array.from(
                new Set([
                  ...ids,
                  ...options.flatMap((o) =>
                    o.release_id ? [o.release_id] : [],
                  ),
                ]),
              ).map((id) => {
                const o = options.find((x) => x.release_id === id);
                const index = ids.indexOf(id);
                return (
                  <div className="flex items-center gap-2" key={id}>
                    <Label>
                      <Checkbox
                        disabled={busy}
                        checked={index >= 0}
                        onCheckedChange={(v) =>
                          edit(
                            v === true
                              ? [...ids, id]
                              : ids.filter((x) => x !== id),
                          )
                        }
                      />
                      {o
                        ? o.provider_name + " · " + o.provider_model_id
                        : "已失效线路 " + id}
                    </Label>
                    {index >= 0 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={index === 0 || busy}
                        onClick={() => {
                          const n = [...ids];
                          [n[index - 1], n[index]] = [n[index], n[index - 1]];
                          edit(n);
                        }}
                      >
                        优先级 {index + 1} · 上移
                      </Button>
                    )}
                  </div>
                );
              })}
              {!!data.recent?.length && (
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button type="button" variant="ghost" size="sm">
                      最近派发记录
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <ul>
                      {data.recent.map((a) => (
                        <li key={a.operation_id + "-" + a.attempt}>
                          {a.operation_id} · 第 {a.attempt} 次 ·{" "}
                          {a.provider_name} · 尾号 {a.key_suffix} ·{" "}
                          {a.reason === "dial_not_sent"
                            ? "未建立连接"
                            : a.reason}
                        </li>
                      ))}
                    </ul>
                  </CollapsibleContent>
                </Collapsible>
              )}
              {draft && (
                <>
                  {ids.length > 0 && (
                    <Label>
                      <Checkbox
                        checked={review}
                        onCheckedChange={(v) => setReview(v === true)}
                      />
                      已核对上游型号等价、素材授权及数据处理范围
                    </Label>
                  )}
                  <Button
                    type="button"
                    size="sm"
                    disabled={busy || (ids.length > 0 && !review)}
                    onClick={() => void publish()}
                  >
                    发布线路
                  </Button>
                </>
              )}
            </>
          )}
          {message && <p role="status">{message}</p>}
        </div>
      )}
    </section>
  );
}

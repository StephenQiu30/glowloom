"use client";
import { useId, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { ApiError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { BibleWriteDialog } from "./bible-entry-dialog";
import { bibleScopeKey, getBibleDetail, listBible } from "./bible-queries";
import {
  impactSchema,
  kindLabels,
  type BibleCommand,
  type BibleDetail,
} from "./bible-model";
import type { BibleIdentity } from "./bible-model";
import type { BibleWriter } from "./use-bible-writer";

type Action =
  | "confirm"
  | "delete"
  | "restore"
  | "look_delete"
  | "look_default"
  | "voice_unbind"
  | "merge";
export type BibleActionFrame = {
  action: Action;
  detail: BibleDetail;
  lookId?: string;
};
const labels = {
  confirm: "确认当前设定",
  delete: "回收设定",
  restore: "恢复设定",
  look_delete: "删除造型",
  look_default: "设置默认造型",
  voice_unbind: "解除声音绑定",
  merge: "合并角色身份",
};
export function BibleActionDialog({
  identity,
  frame,
  writer,
  onClose,
  onCloseAutoFocus,
}: {
  identity: BibleIdentity;
  frame: BibleActionFrame;
  writer: BibleWriter;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const id = useId(),
    [detail, setDetail] = useState(frame.detail),
    [targetId, setTargetId] = useState(""),
    [preparedTarget, setPreparedTarget] = useState<BibleDetail>(),
    [reviewed, setReviewed] = useState<string>(),
    [proof, setProof] = useState<ReturnType<typeof impactSchema.parse>>(),
    [error, setError] = useState<string>();
  const isMerge = frame.action === "merge";
  const targets = useInfiniteQuery({
    queryKey: [...bibleScopeKey(identity), "merge-targets"],
    queryFn: ({ signal, pageParam }) =>
      listBible(
        identity.projectId,
        "character",
        { cursor: pageParam },
        signal,
        identity,
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
    enabled: isMerge,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const target = useQuery({
    queryKey: [...bibleScopeKey(identity), "merge-target", targetId],
    queryFn: ({ signal }) =>
      getBibleDetail(identity, "character", targetId, signal),
    enabled: isMerge && Boolean(targetId),
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const selected =
    preparedTarget?.head.id === targetId
      ? preparedTarget
      : target.isSuccess && target.isFetchedAfterMount
        ? target.data
        : undefined;
  const reviewKey = JSON.stringify([
    detail.head.id,
    detail.head.revision,
    detail.current.id,
    detail.current.content_sha256,
    selected?.head.id,
    selected?.head.revision,
    selected?.current.id,
    selected?.current.content_sha256,
    proof,
  ]);
  const active = !detail.head.deleted && !detail.head.redirect_id;
  const look =
    detail.current.kind === "character"
      ? detail.current.character.looks.find((item) => item.id === frame.lookId)
      : undefined;
  const available =
    frame.action === "restore"
      ? detail.head.deleted && !detail.head.redirect_id
      : active &&
        (frame.action === "look_delete"
          ? Boolean(look && !look.default)
          : frame.action === "look_default"
            ? Boolean(look && !look.default)
            : frame.action === "voice_unbind"
              ? detail.current.kind === "character" &&
                Boolean(detail.current.character.voice)
              : true);
  const valid =
    available &&
    (!isMerge ||
      Boolean(
        detail.head.kind === "character" &&
        detail.head.confirmed_version_id === detail.current.id &&
        selected?.head.kind === "character" &&
        selected.head.confirmed_version_id === selected.current.id &&
        !selected.head.deleted &&
        !selected.head.redirect_id &&
        selected.head.id !== detail.head.id,
      ));
  const cause = writer.rejected?.cause;
  const requiredProof =
    cause instanceof ApiError && cause.code === "impact_acknowledgment_required"
      ? impactSchema.safeParse(cause.meta?.impact)
      : undefined;
  async function prepare() {
    const latest = writer.latest?.detail;
    if (!latest || latest.head.id !== detail.head.id) return;
    const latestTarget = writer.latest?.target;
    if (isMerge && latestTarget?.head.id !== targetId) return;
    if (await writer.discardRejected()) {
      setDetail(latest);
      setPreparedTarget(latestTarget);
      setReviewed(undefined);
      if (requiredProof?.success) setProof(requiredProof.data);
      else setProof(undefined);
    }
  }
  async function submit() {
    if (reviewed !== reviewKey || !valid) return;
    const base = {
      expected_revision: detail.head.revision,
      ...(proof ? { acknowledged_impact: proof } : {}),
    };
    let command: BibleCommand;
    if (frame.action === "merge") {
      if (!selected) return;
      command = {
        action: "merge",
        kind: "character",
        id: detail.head.id,
        body: {
          ...base,
          target_id: selected.head.id,
          expected_target_revision: selected.head.revision,
        },
      };
    } else if (
      frame.action === "look_default" ||
      frame.action === "look_delete"
    ) {
      if (!frame.lookId) return;
      command = {
        action: frame.action,
        kind: "character",
        id: detail.head.id,
        lookId: frame.lookId,
        body: base,
      };
    } else if (frame.action === "voice_unbind")
      command = {
        action: "voice_unbind",
        kind: "character",
        id: detail.head.id,
        body: { expected_revision: detail.head.revision },
      };
    else
      command = {
        action: frame.action,
        kind: detail.head.kind,
        id: detail.head.id,
        body: base,
      };
    try {
      await writer.submit(command);
    } catch {
      setError("当前确认输入无效，请核对原身份和当前版本。");
    }
  }
  return (
    <BibleWriteDialog
      title={labels[frame.action]}
      writer={writer}
      dirty={Boolean(reviewed || targetId)}
      onClose={onClose}
      onCloseAutoFocus={onCloseAutoFocus}
      onPrepareLatest={prepare}
    >
      <p className="text-sm break-all">
        {kindLabels[detail.head.kind]} {detail.head.id} · 当前版本{" "}
        {detail.head.revision} · 不变版本 {detail.current.id} · SHA{" "}
        {detail.current.content_sha256}
      </p>
      {frame.action === "confirm" && (
        <p>
          明确确认当前完整内容。后续修改产生新版本，已有台词与历史采纳仍保留原确认版本。
        </p>
      )}
      {frame.action === "delete" && (
        <p>
          回收保留身份与不可变历史，之后可按该原身份恢复；当前影响必须由正式下游事实核验。
        </p>
      )}
      {frame.action === "look_delete" && (
        <p>
          删除当前造型前必须由正式下游核验使用影响。历史版本仍保留该造型和参考图。
        </p>
      )}
      {frame.action === "voice_unbind" && (
        <p>仅解除当前角色声音，历史版本与原音频样本保持。</p>
      )}
      {isMerge && (
        <div className="space-y-3">
          <Label htmlFor={`${id}-target`}>合并到已确认角色</Label>
          <select
            id={`${id}-target`}
            disabled={writer.locked}
            className="h-10 w-full rounded-md bg-muted px-2"
            value={targetId}
            onChange={(event) => {
              setTargetId(event.target.value);
              setPreparedTarget(undefined);
              setReviewed(undefined);
            }}
          >
            <option value="">选择另一个已确认角色</option>
            {targets.data?.pages
              .flatMap((page) => page.entries)
              .filter(
                (item) =>
                  item.head.id !== detail.head.id &&
                  item.head.confirmed_version_id ===
                    item.head.current_version_id,
              )
              .map((item) => (
                <option key={item.head.id} value={item.head.id}>
                  {item.name} · {item.head.id}
                </option>
              ))}
          </select>
          {targets.hasNextPage && (
            <Button
              variant="outline"
              disabled={writer.locked || targets.isFetchingNextPage}
              onClick={() => void targets.fetchNextPage()}
            >
              读取更多已确认角色
            </Button>
          )}
          {(targets.isError || target.isError) && (
            <p role="alert">目标角色当前事实无法读取，不能合并。</p>
          )}
          {selected && (
            <p className="break-all">
              目标当前版本 {selected.head.revision} · 不变版本{" "}
              {selected.current.id} · SHA {selected.current.content_sha256}
            </p>
          )}
          <p>
            合并保存原身份到目标的重定向。两边完整造型与不可变历史仍按原身份保留，不改写旧版本；影响核验不可用时无法完成。
          </p>
        </div>
      )}
      {proof && (
        <p className="break-all">
          服务端实际影响证明 SHA {proof.sha256} · 影响版本 {proof.revision}
          。此证明不表示影响数量为零。
        </p>
      )}
      {!available && (
        <p role="alert">
          当前身份或造型状态已不适用于此动作，请关闭后读取当前事实。
        </p>
      )}
      <div className="flex items-start gap-2">
        <Checkbox
          id={`${id}-review`}
          checked={reviewed === reviewKey}
          disabled={writer.locked || !valid}
          onCheckedChange={(checked) =>
            setReviewed(checked === true ? reviewKey : undefined)
          }
        />
        <Label htmlFor={`${id}-review`}>
          我已审核以上完整版本和本次明确动作
          {proof ? "，并确认本次实际影响证明" : ""}。
        </Label>
      </div>
      <Button
        disabled={writer.locked || reviewed !== reviewKey || !valid}
        onClick={() => void submit()}
      >
        {labels[frame.action]}
      </Button>
      {error && <p role="alert">{error}</p>}
    </BibleWriteDialog>
  );
}

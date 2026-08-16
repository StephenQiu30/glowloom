"use client";

import { AlertTriangle, X } from "lucide-react";
import { type FormEvent, useState } from "react";

import { Badge } from "@/components/ui/badge";
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

function ImpactSummary({ impact }: { impact: API.AssetImpactResponse }) {
  const { summary } = impact;
  return (
    <div className="grid gap-4">
      <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-amber-900">
        <div className="flex items-start gap-3">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium">影响 {summary.shot_count} 个分镜</p>
            <p className="mt-1 text-xs leading-5 text-amber-800/80">
              涉及 {summary.episode_count} 集、{summary.spec_version_count} 个规格版本、
              {summary.prompt_snapshot_count} 份提示词快照和 {summary.active_task_count}
              个进行中任务。
            </p>
          </div>
        </div>
      </div>
      {impact.shots.length > 0 ? (
        <div>
          <p className="text-sm font-medium">受影响分镜</p>
          <ul className="mt-2 grid max-h-40 gap-2 overflow-y-auto">
            {impact.shots.map((shot) => (
              <li
                className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm"
                key={shot.shot_id}
              >
                <span className="truncate">{shot.shot_title}</span>
                <Badge variant="outline">{shot.slot_keys.length} 个引用位</Badge>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <p className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800">
          当前没有分镜、提示词快照或进行中任务受影响。
        </p>
      )}
      <p className="text-xs leading-5 text-slate-500">
        历史资产版本、分镜规格和生成请求不会被覆盖或删除；确认时会再次校验影响摘要。
      </p>
    </div>
  );
}

export function AssetImpactDialog({
  confirmLabel,
  description,
  impact,
  isApplying,
  isLoading,
  onConfirm,
  onOpenChange,
  open,
  title,
}: {
  confirmLabel: string;
  description: string;
  impact?: API.AssetImpactResponse;
  isApplying: boolean;
  isLoading: boolean;
  onConfirm: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  title: string;
}) {
  return (
    <DialogRoot onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-2xl" showCloseButton={false}>
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
          <div className="mt-6">
            {isLoading ? (
              <p className="text-sm text-slate-500">正在锁定并计算最新影响…</p>
            ) : impact ? (
              <ImpactSummary impact={impact} />
            ) : null}
          </div>
          <div className="mt-6 flex justify-end gap-2">
            <DialogClose asChild>
              <Button type="button" variant="outline">取消</Button>
            </DialogClose>
            <Button
              disabled={!impact || isLoading || isApplying}
              onClick={() => void onConfirm()}
              type="button"
              variant="destructive"
            >
              {isApplying ? "提交中…" : confirmLabel}
            </Button>
          </div>
      </DialogContent>
    </DialogRoot>
  );
}

export function RenameAssetDialog({
  asset,
  isApplying,
  isLoading,
  onApply,
  onOpenChange,
  onPreflight,
  open,
}: {
  asset: API.AssetResponse;
  isApplying: boolean;
  isLoading: boolean;
  onApply: (newName: string, impact: API.AssetImpactResponse) => Promise<boolean>;
  onOpenChange: (open: boolean) => void;
  onPreflight: (newName: string) => Promise<API.AssetImpactResponse | undefined>;
  open: boolean;
}) {
  const [newName, setNewName] = useState(asset.name);
  const [impact, setImpact] = useState<API.AssetImpactResponse>();

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = newName.trim();
    if (!impact) {
      setImpact(await onPreflight(normalized));
      return;
    }
    if (await onApply(normalized, impact)) {
      setImpact(undefined);
      onOpenChange(false);
    }
  }

  function changeOpen(value: boolean) {
    if (!value) {
      setNewName(asset.name);
      setImpact(undefined);
    }
    onOpenChange(value);
  }

  return (
    <DialogRoot onOpenChange={changeOpen} open={open}>
      <DialogContent className="max-w-2xl" showCloseButton={false}>
          <div className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle className="text-xl font-semibold tracking-tight">
                重命名资产
              </DialogTitle>
              <DialogDescription className="mt-1 text-sm leading-6 text-muted-foreground">
                名称会建立新修订，旧名称保留为别名；稳定资产 ID 和历史引用不变。
              </DialogDescription>
            </div>
            <DialogClose asChild>
              <Button aria-label="关闭" size="icon" variant="ghost"><X /></Button>
            </DialogClose>
          </div>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="assetNewName">新资产名称</Label>
              <Input
                id="assetNewName"
                maxLength={200}
                onChange={(event) => {
                  setNewName(event.target.value);
                  setImpact(undefined);
                }}
                required
                value={newName}
              />
            </div>
            {impact ? <ImpactSummary impact={impact} /> : null}
            <div className="flex justify-end gap-2">
              <DialogClose asChild>
                <Button type="button" variant="outline">取消</Button>
              </DialogClose>
              <Button
                disabled={isLoading || isApplying || newName.trim() === asset.name}
                type="submit"
              >
                {isLoading
                  ? "检查中…"
                  : isApplying
                    ? "提交中…"
                    : impact
                      ? "确认重命名"
                      : "检查影响"}
              </Button>
            </div>
          </form>
      </DialogContent>
    </DialogRoot>
  );
}

"use client";

import Link from "next/link";
import { Globe, MoreHorizontal, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";

export function regionLabel(region?: string) {
  return region === "domestic"
    ? "国内"
    : region === "overseas"
      ? "海外"
      : "区域未知";
}
export function checkLabel(result?: string) {
  const labels: Record<string, string> = {
    ok: "检查通过",
    auth_failed: "鉴权未通过",
    unreachable: "无法连接",
    timeout: "检查超时",
    unsupported: "不支持检查",
  };
  return result ? (labels[result] ?? "结果未知") : "未检查";
}
function KeySummary({
  credential,
}: {
  credential?: API.ProviderCredentialSummary;
}) {
  return (
    <span>
      {credential
        ? `${credential.status === "active" ? "已配置" : credential.status === "draining" ? "仅供恢复" : credential.status === "disabled" ? "已停用" : "状态未知"}${credential.last4 ? ` · ••••${credential.last4}` : ""}`
        : "未配置"}
    </span>
  );
}
function ConnectionMenu({
  item,
  onSelect,
  onDisable,
}: {
  item: API.ProviderListItem;
  onSelect: (id: string) => void;
  onDisable: (provider: API.CreatedProvider) => void;
}) {
  const p = item.provider;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="max-md:size-11"
          aria-label={`${p?.name ?? "连接"}的操作`}
        >
          <MoreHorizontal data-icon="inline-start" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuItem onSelect={() => onSelect(p!.id!)}>
            查看详情
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href={`/models?provider=${encodeURIComponent(p!.id!)}`}>
              模型管理
            </Link>
          </DropdownMenuItem>
          {p?.status === "active" ? (
            <DropdownMenuItem onSelect={() => onDisable(p)}>
              停用连接
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
export function ConnectionList({
  items,
  onSelect,
  onDisable,
  pendingCredential,
}: {
  pendingCredential?: string;
  items: API.ProviderListItem[];
  onSelect: (id: string) => void;
  onDisable: (provider: API.CreatedProvider) => void;
}) {
  return (
    <section aria-label="连接列表" className="grid gap-4 sm:grid-cols-2">
      {items.map((item) => {
        const p = item.provider!;
        const Icon = p.adapter_key === "volcengine_ark" ? Server : Globe;
        const pending = pendingCredential === p.id;
        const status = pending
          ? "保存待确认"
          : p.status === "disabled"
            ? "已停用"
            : p.status !== "active"
              ? "状态未知"
              : !item.credential
                ? "待配置 Key"
                : item.credential.status === "disabled"
                  ? "Key 已停用"
                  : item.credential.status === "draining"
                    ? "Key 仅供恢复"
                    : item.credential.status !== "active"
                      ? "Key 状态未知"
                      : checkLabel(item.credential.last_test_result);
        return (
          <article
            key={p.id}
            className="flex min-w-0 flex-col gap-5 rounded-xl border bg-card p-5"
          >
            <div className="flex items-start justify-between gap-2">
              <Button
                variant="ghost"
                className="-mt-2 -ml-2 h-auto min-w-0 flex-1 justify-start gap-3 p-2 text-left whitespace-normal"
                onClick={() => onSelect(p.id!)}
                aria-label={`查看 ${p.name}`}
              >
                <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted/40">
                  <Icon className="size-5" />
                </span>
                <span className="flex min-w-0 flex-col gap-1">
                  <span className="font-medium break-words">{p.name}</span>
                  <span className="text-xs font-normal text-muted-foreground">
                    {regionLabel(p.region)}
                  </span>
                </span>
              </Button>
              <ConnectionMenu
                item={item}
                onSelect={onSelect}
                onDisable={onDisable}
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Badge variant="secondary">{status}</Badge>
              <span className="text-xs text-muted-foreground tabular-nums">
                {item.model_count === undefined
                  ? "模型数量未知"
                  : `${item.model_count} 个模型`}
              </span>
            </div>
            <div className="flex items-center justify-between gap-2 border-t pt-3 text-xs text-muted-foreground">
              <span>API Key</span>
              {pending ? (
                <span>待确认</span>
              ) : (
                <KeySummary credential={item.credential} />
              )}
            </div>
          </article>
        );
      })}
    </section>
  );
}

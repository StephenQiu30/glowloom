"use client";

import Link from "next/link";
import { MoreHorizontal } from "lucide-react";
import type { AdminTableColumn } from "@/components/governance/admin-table";
import type { ConnectionRow } from "@/components/governance/batch-actions";
import { useIsMobile } from "@/hooks/use-mobile";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { providerLabel } from "./provider-label";
import { providerConnectionName } from "@/lib/provider-connection-name";
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
        ? credential.status === "active"
          ? "已配置"
          : credential.status === "draining"
            ? "仅供恢复"
            : credential.status === "disabled"
              ? "已停用"
              : "状态未知"
        : "未配置"}
    </span>
  );
}
export type ConnectionFilter = "name" | "adapter_key" | "status";

export function useConnectionColumns({
  adapters,
  onSelect,
  onDelete,
  disabled,
}: {
  adapters?: API.AdapterMetadata[];
  pendingCredential?: string;
  onSelect: (id: string, trigger: HTMLElement, rename?: boolean) => void;
  onDelete?: (row: ConnectionRow, trigger: HTMLElement | null) => void;
  disabled?: boolean;
}) {
  const mobile = useIsMobile();
  const columns: AdminTableColumn<ConnectionRow, ConnectionFilter>[] = [
    {
      id: "name",
      header: "连接名称",
      filter: { name: "name", type: "text", placeholder: "输入连接名称" },
      cell: ({ row }) => {
        const name = providerConnectionName(row.original.provider);
        return (
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              {mobile ? (
                <ProviderLogo
                  adapterKey={row.original.provider?.adapter_key}
                  metadata={adapters?.find(
                    (item) =>
                      item.adapter_key === row.original.provider?.adapter_key,
                  )}
                />
              ) : null}
              <Button
                variant="link"
                className="h-auto min-w-0 justify-start p-0 text-left whitespace-normal text-foreground"
                disabled={disabled}
                data-connection-trigger={`${row.original.id}:overview`}
                aria-label={`查看 ${name}`}
                onClick={(event) =>
                  onSelect(row.original.id, event.currentTarget)
                }
              >
                {name}
              </Button>
            </div>
            <span className="text-xs text-muted-foreground">
              {mobile
                ? `${providerLabel(row.original.provider?.adapter_key, adapters)} · `
                : ""}
              {regionLabel(row.original.provider?.region)}
              {mobile ? (
                <>
                  {" "}
                  ·{" "}
                  {disabled ? (
                    `${row.original.available_model_count ?? 0}/${row.original.model_count ?? 0} 个模型`
                  ) : (
                    <Link
                      className="underline underline-offset-2"
                      href={`/models?provider=${encodeURIComponent(row.original.id)}`}
                    >
                      {row.original.available_model_count ?? 0}/
                      {row.original.model_count ?? 0} 个模型
                    </Link>
                  )}
                </>
              ) : null}
            </span>
          </div>
        );
      },
    },
    {
      id: "adapter",
      header: "供应商",
      settings: { hideOnMobile: true },
      filter: {
        name: "adapter_key",
        type: "select",
        options: (adapters ?? []).flatMap(({ adapter_key }) =>
          adapter_key
            ? [
                {
                  value: adapter_key,
                  label: providerLabel(adapter_key, adapters),
                },
              ]
            : [],
        ),
      },
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <ProviderLogo
            adapterKey={row.original.provider?.adapter_key}
            metadata={adapters?.find(
              (item) => item.adapter_key === row.original.provider?.adapter_key,
            )}
          />
          <span>
            {providerLabel(row.original.provider?.adapter_key, adapters)}
          </span>
        </div>
      ),
    },
    {
      id: "models",
      header: "接入模型",
      settings: { hideOnMobile: true },
      cell: ({ row }) =>
        disabled ? (
          `${row.original.available_model_count ?? 0}/${row.original.model_count ?? 0} 个模型`
        ) : (
          <Button asChild variant="link" className="h-auto p-0">
            <Link
              href={`/models?provider=${encodeURIComponent(row.original.id)}`}
            >
              {row.original.available_model_count ?? 0}/
              {row.original.model_count ?? 0} 个模型
            </Link>
          </Button>
        ),
    },
    {
      id: "status",
      header: "状态",
      filter: {
        name: "status",
        type: "select",
        options: [
          { value: "available", label: "可用" },
          { value: "unavailable", label: "不可用" },
        ],
      },
      cell: ({ row }) => (
        <Badge variant="secondary">
          {row.original.available === true ? "可用" : "不可用"}
        </Badge>
      ),
    },
    {
      id: "credential",
      header: "配置状态",
      settings: { defaultVisible: false, hideOnMobile: true },
      cell: ({ row }) => <KeySummary credential={row.original.credential} />,
    },
    {
      id: "check",
      header: "最近检查",
      settings: { defaultVisible: false, hideOnMobile: true },
      cell: ({ row }) =>
        row.original.credential?.status === "disabled"
          ? "Key 已停用"
          : row.original.credential?.status === "draining"
            ? "Key 仅供恢复"
            : checkLabel(row.original.credential?.last_test_result),
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={(event) => onSelect(row.original.id, event.currentTarget)}
            className="max-md:h-11"
            data-connection-trigger={`${row.original.id}:manage`}
            aria-label={`管理 ${providerConnectionName(row.original.provider)}`}
          >
            {row.original.provider?.status === "archived" ||
            row.original.credential?.status === "active"
              ? "管理"
              : "配置"}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                id={`provider-actions-${row.original.id}`}
                data-connection-trigger={`${row.original.id}:actions`}
                variant="ghost"
                size="icon-sm"
                className="max-md:size-11"
                disabled={disabled}
                aria-label={`${providerConnectionName(row.original.provider)}的更多操作`}
              >
                <MoreHorizontal />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  onSelect={() => {
                    const trigger = document.getElementById(
                      `provider-actions-${row.original.id}`,
                    );
                    if (trigger) onSelect(row.original.id, trigger, true);
                  }}
                >
                  编辑名称
                </DropdownMenuItem>
                {onDelete ? (
                  <DropdownMenuItem
                    onSelect={() =>
                      onDelete(
                        row.original,
                        document.getElementById(
                          `provider-actions-${row.original.id}`,
                        ),
                      )
                    }
                  >
                    删除连接
                  </DropdownMenuItem>
                ) : null}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ),
    },
  ];
  return columns;
}

"use client";

import Link from "next/link";
import { useState } from "react";
import { ColumnMenu } from "@/components/governance/column-menu";
import type { Dispatch, SetStateAction } from "react";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import type { ConnectionRow } from "@/components/governance/batch-actions";
import { useIsMobile } from "@/hooks/use-mobile";
import { MoreHorizontal } from "lucide-react";
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
  disabled,
  onSelect,
  onDisable,
}: {
  item: API.ProviderListItem;
  disabled?: boolean;
  onSelect: (id: string) => void;
  onDisable: (provider: API.CreatedProvider) => void;
}) {
  const p = item.provider;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          disabled={disabled}
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
  selection,
  disabled,
}: {
  pendingCredential?: string;
  items: ConnectionRow[];
  onSelect: (id: string) => void;
  onDisable: (provider: API.CreatedProvider) => void;
  selection: {
    selected: ConnectionRow[];
    onChange: Dispatch<SetStateAction<ConnectionRow[]>>;
  };
  disabled?: boolean;
}) {
  const mobile = useIsMobile();
  const [visibility, setVisibility] = useState<Record<string, boolean>>({});
  const columns: DataTableColumn<ConnectionRow>[] = [
    {
      id: "name",
      header: "连接",
      cell: ({ row }) => (
        <div className="flex flex-col gap-1">
          <span className="break-all whitespace-normal">
            {row.original.provider?.name}
          </span>
          <span className="text-xs text-muted-foreground">
            {regionLabel(row.original.provider?.region)} ·{" "}
            {row.original.model_count ?? 0} 个模型
          </span>
        </div>
      ),
    },
    {
      id: "adapter",
      header: "服务",
      cell: ({ row }) => row.original.provider?.adapter_key ?? "—",
    },
    {
      id: "status",
      header: "状态",
      cell: ({ row }) => (
        <Badge variant="secondary">
          {pendingCredential === row.original.id
            ? "保存待确认"
            : row.original.provider?.status === "active"
              ? "启用"
              : "已停用"}
        </Badge>
      ),
    },
    {
      id: "credential",
      header: "凭据",
      cell: ({ row }) => <KeySummary credential={row.original.credential} />,
    },
    {
      id: "check",
      header: "最近检查",
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
            variant="ghost"
            disabled={disabled}
            onClick={() => onSelect(row.original.id)}
            aria-label={`查看 ${row.original.provider?.name}`}
          >
            查看
          </Button>
          <ConnectionMenu
            disabled={disabled}
            item={row.original}
            onSelect={onSelect}
            onDisable={onDisable}
          />
        </div>
      ),
    },
  ];
  return (
    <div className="flex flex-col gap-2">
      {!mobile ? (
        <div className="self-end">
          <ColumnMenu
            columns={[
              { id: "adapter", label: "服务" },
              { id: "credential", label: "凭据" },
              { id: "check", label: "最近检查" },
            ]}
            visibility={visibility}
            onChange={setVisibility}
          />
        </div>
      ) : null}
      <DataTable
        density="comfortable"
        label="连接列表"
        columns={columns}
        data={items}
        emptyTitle="没有匹配的连接"
        columnVisibility={{
          adapter: !mobile && visibility.adapter !== false,
          credential: !mobile && visibility.credential !== false,
          check: !mobile && visibility.check !== false,
        }}
        selection={{
          ...selection,
          getLabel: (r) => r.provider?.name ?? r.id,
          pageLabel: "选择当前页全部连接",
          disabled,
        }}
      />
    </div>
  );
}

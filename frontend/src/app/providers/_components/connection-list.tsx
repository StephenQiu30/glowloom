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
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import {
  Item,
  ItemGroup,
  ItemMedia,
  ItemContent,
  ItemTitle,
  ItemDescription,
  ItemActions,
} from "@/components/ui/item";

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
    <section aria-label="连接列表">
      <div className="hidden md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>服务</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>模型</TableHead>
              <TableHead>
                <span className="sr-only">操作</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => {
              const p = item.provider!;
              const Icon = p.adapter_key === "volcengine_ark" ? Server : Globe;
              return (
                <TableRow key={p.id}>
                  <TableCell className="w-2/5 whitespace-normal">
                    <Button
                      variant="ghost"
                      className="h-auto min-h-16 w-full justify-start gap-3 px-2 py-3 whitespace-normal"
                      onClick={() => onSelect(p.id!)}
                      aria-label={`查看 ${p.name}`}
                    >
                      <Icon data-icon="inline-start" />
                      <span className="flex min-w-0 flex-col items-start gap-1">
                        <span className="break-words">{p.name}</span>
                        <span className="text-xs text-muted-foreground">
                          {p.adapter_key} · {regionLabel(p.region)}
                        </span>
                      </span>
                    </Button>
                  </TableCell>
                  <TableCell className="whitespace-normal">
                    <div className="flex flex-col items-start gap-2">
                      <Badge variant="secondary">
                        {pendingCredential === p.id
                          ? "保存待确认"
                          : checkLabel(item.credential?.last_test_result)}
                      </Badge>
                      <span className="text-xs text-muted-foreground">
                        {p.status === "active"
                          ? "已启用"
                          : p.status === "disabled"
                            ? "已停用"
                            : "状态未知"}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-normal">
                    {pendingCredential === p.id ? (
                      "待确认"
                    ) : (
                      <KeySummary credential={item.credential} />
                    )}
                  </TableCell>
                  <TableCell className="whitespace-normal">
                    {item.model_count === undefined
                      ? "数量未知"
                      : `${item.model_count} 个模型`}
                  </TableCell>
                  <TableCell>
                    <ConnectionMenu
                      item={item}
                      onSelect={onSelect}
                      onDisable={onDisable}
                    />
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
      <ItemGroup className="md:hidden">
        {items.map((item) => {
          const p = item.provider!;
          const Icon = p.adapter_key === "volcengine_ark" ? Server : Globe;
          return (
            <Item key={p.id} variant="outline">
              <ItemMedia variant="icon">
                <Icon />
              </ItemMedia>
              <ItemContent className="min-w-0">
                <ItemTitle>
                  <Button
                    variant="ghost"
                    className="h-auto min-h-11 justify-start px-0 whitespace-normal"
                    onClick={() => onSelect(p.id!)}
                    aria-label={`查看 ${p.name}`}
                  >
                    {p.name}
                  </Button>
                </ItemTitle>
                <ItemDescription>
                  {regionLabel(p.region)} ·{" "}
                  {p.status === "active"
                    ? "已启用"
                    : p.status === "disabled"
                      ? "已停用"
                      : "状态未知"}
                </ItemDescription>
                <ItemDescription>
                  {pendingCredential === p.id ? (
                    "待确认"
                  ) : (
                    <KeySummary credential={item.credential} />
                  )}
                </ItemDescription>
                <ItemDescription>
                  {item.model_count === undefined
                    ? "模型数量未知"
                    : `${item.model_count} 个模型`}
                </ItemDescription>
                <Badge variant="secondary">
                  {pendingCredential === p.id
                    ? "保存待确认"
                    : checkLabel(item.credential?.last_test_result)}
                </Badge>
              </ItemContent>
              <ItemActions>
                <ConnectionMenu
                  item={item}
                  onSelect={onSelect}
                  onDisable={onDisable}
                />
              </ItemActions>
            </Item>
          );
        })}
      </ItemGroup>
    </section>
  );
}

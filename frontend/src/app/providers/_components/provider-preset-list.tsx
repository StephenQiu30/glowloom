"use client";

import { saveAs } from "file-saver";
import { MoreHorizontal } from "lucide-react";
import { toast } from "sonner";
import { exportProviderPreset } from "@/api/settings";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ContentState } from "@/components/layout/content-state";
import { canConnectPreset, providerLabel } from "./provider-label";
import { ProviderLogo } from "./provider-logo";

export type PresetStateAction = "archive" | "emergency";

export function ProviderPresetList({
  items,
  adapters,
  busy,
  filtered,
  onEdit,
  onState,
}: {
  items: API.Preset[];
  adapters: API.AdapterMetadata[];
  busy: boolean;
  filtered: boolean;
  onEdit: (preset: API.Preset) => void;
  onState: (preset: API.Preset, action: PresetStateAction) => void;
}) {
  if (!items.length) {
    return (
      <ContentState title={filtered ? "没有匹配的预设" : "还没有供应商预设"} />
    );
  }
  return (
    <div className="min-w-0 overflow-hidden rounded-lg border">
      <Table aria-label="供应商预设列表">
        <TableHeader>
          <TableRow>
            <TableHead>预设名称</TableHead>
            <TableHead className="hidden sm:table-cell">供应商</TableHead>
            <TableHead className="hidden sm:table-cell">来源</TableHead>
            <TableHead className="hidden sm:table-cell">版本</TableHead>
            <TableHead>状态</TableHead>
            <TableHead className="text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((preset) => {
            const adapter = preset.document?.config?.adapter_key;
            const supplier = providerLabel(adapter, adapters);
            const name = preset.document?.name ?? supplier;
            const source = preset.origin === "builtin" ? "内置" : "自定义";
            const version = `v${preset.version}`;
            return (
              <TableRow key={preset.id}>
                <TableCell className="min-w-0 whitespace-normal">
                  <div className="flex items-center gap-2">
                    <ProviderLogo
                      adapterKey={adapter}
                      metadata={adapters.find(
                        (item) => item.adapter_key === adapter,
                      )}
                    />
                    <div className="min-w-0 break-words">
                      <div>{name}</div>
                      <div className="text-xs text-muted-foreground sm:hidden">
                        {supplier} · {source} · {version}
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell className="hidden sm:table-cell">
                  {supplier}
                </TableCell>
                <TableCell className="hidden sm:table-cell">{source}</TableCell>
                <TableCell className="hidden sm:table-cell">
                  {version}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {preset.archived ? (
                      <Badge variant="secondary">已归档</Badge>
                    ) : null}
                    {preset.emergency_disabled ? (
                      <Badge variant="destructive">紧急停用</Badge>
                    ) : null}
                    {!preset.archived && !preset.emergency_disabled ? (
                      <Badge variant="secondary">
                        {canConnectPreset(preset) ? "可用" : "仅供维护"}
                      </Badge>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="max-md:size-11"
                        aria-label={`${name}预设的操作`}
                        disabled={busy}
                      >
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuGroup>
                        <DropdownMenuItem onSelect={() => onEdit(preset)}>
                          {preset.origin === "builtin"
                            ? "复制并编辑"
                            : "编辑新版本"}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onSelect={() => {
                            void exportProviderPreset({ id: preset.id! })
                              .then((result) =>
                                saveAs(
                                  new Blob([JSON.stringify(result, null, 2)], {
                                    type: "application/json",
                                  }),
                                  `provider-preset-${preset.id}.json`,
                                ),
                              )
                              .catch(() =>
                                toast.error("预设导出失败，请重试。"),
                              );
                          }}
                        >
                          导出
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onSelect={() => onState(preset, "archive")}
                        >
                          {preset.archived ? "恢复预设" : "归档预设"}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onSelect={() => onState(preset, "emergency")}
                        >
                          {preset.emergency_disabled
                            ? "解除紧急停用"
                            : "紧急停用"}
                        </DropdownMenuItem>
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

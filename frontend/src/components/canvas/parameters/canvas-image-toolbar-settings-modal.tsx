import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

import { Ellipsis, Image as ImageIcon, Settings2 } from "lucide-react";
import { useId, useMemo, type ReactNode } from "react";

import type { ImageQuickToolId } from "../lib/canvas-image-toolbar-tools";

export type ImageToolbarSettingsTool = {
  id: ImageQuickToolId;
  title: string;
  label: string;
  icon: ReactNode;
  active?: boolean;
  danger?: boolean;
};

type PreviewTool =
  | ImageToolbarSettingsTool
  | {
      id: "more";
      title: string;
      label: string;
      icon: ReactNode;
      active?: boolean;
      danger?: boolean;
    };

export function ImageToolSettingsModal({
  open,
  tools,
  selectedIds,
  showLabels,
  onToggle,
  onShowLabelsChange,
  onCancel,
  onSave,
}: {
  open: boolean;
  tools: ImageToolbarSettingsTool[];
  selectedIds: ImageQuickToolId[];
  showLabels: boolean;
  onToggle: (id: ImageQuickToolId, visible: boolean) => void;
  onShowLabelsChange: (value: boolean) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  const settingsId = useId();
  const selected = useMemo(() => new Set(selectedIds), [selectedIds]);
  const selectedTools = tools.filter((tool) => selected.has(tool.id));
  const previewTools: PreviewTool[] = [
    ...selectedTools,
    {
      id: "more",
      title: "配置快捷工具",
      label: "更多",
      icon: <Ellipsis className="size-4" />,
      active: true,
    },
  ];

  return (
    <Dialog
      open={open}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <DialogContent
        className={"dialog-settings max-h-dialog w-full overflow-y-auto"}
      >
        <DialogHeader>
          <DialogTitle>{"自定义工具栏"}</DialogTitle>
          <DialogDescription>
            选择图片节点的快捷工具，并预览工具栏显示效果。
          </DialogDescription>
        </DialogHeader>
        <div>
          <Card className="mb-4">
            <CardHeader>
              <CardTitle>
                {
                  <div className={"flex items-center gap-2"}>
                    <Settings2 />
                    {"节点预览"}
                  </div>
                }
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="relative flex min-h-media-preview w-full justify-center pt-20 pb-9">
                <div className="thin-scrollbar absolute top-3 right-2 left-2 flex h-12 items-center gap-1 overflow-x-auto">
                  {previewTools.map((tool) => (
                    <PreviewToolbarItem
                      key={tool.id}
                      tool={tool}
                      showLabels={showLabels}
                    />
                  ))}
                </div>
                <Empty className="max-w-control-group h-48 w-full">
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <ImageIcon />
                    </EmptyMedia>
                    <EmptyTitle>图片节点</EmptyTitle>
                  </EmptyHeader>
                </Empty>
              </div>
            </CardContent>
          </Card>
          <FieldGroup>
            <FieldSet>
              <FieldLegend className="flex items-center gap-2">
                快捷工具
                <Badge variant="secondary">
                  {selectedTools.length}/{tools.length}
                </Badge>
              </FieldLegend>
              <FieldGroup className="gap-3">
                {tools.map((tool) => (
                  <Field key={tool.id} orientation="horizontal">
                    <Checkbox
                      id={`${settingsId}-${tool.id}`}
                      checked={selected.has(tool.id)}
                      onCheckedChange={(checked) =>
                        onToggle(tool.id, checked === true)
                      }
                    />
                    <FieldLabel htmlFor={`${settingsId}-${tool.id}`}>
                      {tool.icon}
                      {tool.label}
                    </FieldLabel>
                  </Field>
                ))}
              </FieldGroup>
            </FieldSet>
            <Field orientation="horizontal">
              <FieldLabel htmlFor={`${settingsId}-labels`}>
                显示按钮文字
              </FieldLabel>
              <Switch
                id={`${settingsId}-labels`}
                checked={showLabels}
                onCheckedChange={onShowLabelsChange}
              />
            </Field>
          </FieldGroup>
        </div>
        <DialogFooter>
          {
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                onClick={onCancel}
                type={"button"}
                variant={"secondary"}
                size="default"
              >
                {"取消"}
              </Button>
              <Button
                onClick={onSave}
                type={"button"}
                variant={"default"}
                size="default"
              >
                {"保存"}
              </Button>
            </div>
          }
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PreviewToolbarItem({
  tool,
  showLabels,
}: {
  tool: PreviewTool;
  showLabels: boolean;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant={tool.danger ? "destructive" : "secondary"}
          tabIndex={0}
          aria-label={tool.title}
          className={cn("h-9 shrink-0 px-2", !showLabels && "justify-center")}
        >
          {tool.icon}
          {showLabels ? (
            <span className="whitespace-nowrap">{tool.label}</span>
          ) : null}
        </Badge>
      </TooltipTrigger>
      <TooltipContent side="top">{tool.title}</TooltipContent>
    </Tooltip>
  );
}

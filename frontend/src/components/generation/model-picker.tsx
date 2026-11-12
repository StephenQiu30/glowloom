"use client";

import { modelAvailableForMode } from "./model-availability";

import { useState, type ComponentProps } from "react";
import { providerConnectionName } from "@/lib/provider-connection-name";
import { Box, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  ComposerSelectTrigger,
  ComposerPopoverContent,
  ComposerToolbarButton,
} from "./composer-toolbar";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Popover, PopoverTrigger } from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectValue,
} from "@/components/ui/select";

export function ModelSelectList({
  compact,
  current,
  children,
  ...props
}: Omit<ComponentProps<typeof SelectContent>, "className"> & {
  compact?: boolean;
  current?: string;
}) {
  return (
    <SelectContent
      side="top"
      align={compact ? "end" : "start"}
      sideOffset={8}
      collisionPadding={16}
      className={
        compact
          ? "composer-model-list model-select-list"
          : "composer-model-list model-select-list model-select-list-detail"
      }
      onEscapeKeyDown={(event) => event.stopPropagation()}
      {...props}
    >
      {!compact && (
        <div className="px-3 pt-2 pb-1 text-xs text-muted-foreground">
          选择模型{current ? `：${current}` : ""}
        </div>
      )}
      <SelectGroup className="flex flex-col gap-2 p-0">{children}</SelectGroup>
    </SelectContent>
  );
}

export function ModelSelectOption({
  name,
  description,
  detailed = false,
  unavailable = false,
  ...props
}: Omit<ComponentProps<typeof SelectItem>, "children" | "className"> & {
  name: string;
  description?: string;
  detailed?: boolean;
  unavailable?: boolean;
}) {
  return (
    <SelectItem textValue={name} className="model-select-option" {...props}>
      <span className="flex min-w-0 items-center gap-3">
        {detailed && (
          <span
            aria-hidden="true"
            className="model-select-symbol flex size-[42px] shrink-0 items-center justify-center rounded-lg border"
          >
            <Box />
          </span>
        )}
        <span className="min-w-0">
          <span className="block truncate">
            {name}
            {unavailable && !description ? " · 当前不可用" : ""}
          </span>
          {description && (
            <span className="mt-0.5 block truncate text-xs text-muted-foreground">
              {description}
            </span>
          )}
        </span>
      </span>
    </SelectItem>
  );
}

/** Selection updates the owning editor and leaves submission to its separate action. */
export function ModelPicker({
  models,
  mode,
  value,
  disabled,
  onSelect,
  onChooseProject,
  compact,
  label = "生成模型",
}: {
  label?: string;
  models: API.ModelResponse[];
  mode?: string | null;
  value: string | null | undefined;
  disabled: boolean;
  onSelect: (model: API.ModelResponse) => void;
  onChooseProject?: () => void;
  compact?: boolean;
}) {
  const [projectPromptOpen, setProjectPromptOpen] = useState(false);
  const choices = models.flatMap((model) => {
    const id = model.current_version?.id;
    if (!id || !modelAvailableForMode(model, mode)) return [];
    return [
      {
        id,
        model,
        name: model.display_name || model.key || "未命名模型",
        unavailable: false,
      },
    ];
  });
  const current = choices.find((choice) => choice.id === value);

  if (onChooseProject) {
    return (
      <Popover open={projectPromptOpen} onOpenChange={setProjectPromptOpen}>
        <PopoverTrigger asChild>
          <ComposerToolbarButton
            type="button"
            variant="ghost"
            size="toolbar"
            disabled={disabled}
            aria-label={label}
            aria-expanded={projectPromptOpen}
            className="max-w-60 justify-start"
          >
            {!compact && <Box data-icon="inline-start" />}
            选择模型
            <ChevronDown
              data-icon="inline-end"
              className="text-muted-foreground"
            />
          </ComposerToolbarButton>
        </PopoverTrigger>
        <ComposerPopoverContent
          width="compact"
          align={compact ? "end" : "start"}
          sideOffset={8}
          collisionPadding={16}
          className="model-select-list p-0"
          aria-label="选择模型"
        >
          <Empty>
            <EmptyHeader>
              <EmptyTitle>选择{label}</EmptyTitle>
              <EmptyDescription>
                请先选择项目，再查看该项目的可用模型。
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button
                type="button"
                variant="secondary"
                onClick={() => {
                  setProjectPromptOpen(false);
                  onChooseProject();
                }}
              >
                选择项目
              </Button>
            </EmptyContent>
          </Empty>
        </ComposerPopoverContent>
      </Popover>
    );
  }

  return (
    <Select
      value={current?.id ?? ""}
      disabled={disabled}
      onValueChange={(next) => {
        const choice = choices.find(({ id }) => id === next);
        if (choice && !choice.unavailable) onSelect(choice.model);
      }}
    >
      <ComposerSelectTrigger aria-label={label} className="max-w-60">
        {!compact && <Box aria-hidden="true" />}
        <SelectValue placeholder="选择模型">{current?.name}</SelectValue>
      </ComposerSelectTrigger>
      <ModelSelectList compact={compact} current={current?.name}>
        {choices.map(({ id, model, name, unavailable }) => (
          <ModelSelectOption
            key={id}
            value={id}
            name={name}
            disabled={unavailable}
            detailed={!compact}
            unavailable={unavailable}
            description={
              compact
                ? undefined
                : unavailable
                  ? "当前不可用"
                  : model.provider_name
                    ? providerConnectionName({ name: model.provider_name })
                    : undefined
            }
          />
        ))}
        {!choices.length && (
          <ModelSelectOption value="unavailable" name="暂无可用模型" disabled />
        )}
      </ModelSelectList>
    </Select>
  );
}

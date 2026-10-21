"use client";

import { useState } from "react";
import { Box, Check, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
} from "@/components/ui/command";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";

/** The same anchored picker is used by homepage and canvas composers. */
export function ModelPicker({
  models,
  value,
  disabled,
  onSelect,
}: {
  models: API.ModelResponse[];
  value: string | null | undefined;
  disabled: boolean;
  onSelect: (model: API.ModelResponse) => void;
}) {
  const [open, setOpen] = useState(false);
  const current = models.find((model) => model.current_version?.id === value);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          aria-label="生成模型"
          aria-expanded={open}
          disabled={disabled}
          className="max-w-60 justify-start pr-2 pl-0"
        >
          <span className="truncate">
            {current?.display_name ?? "选择模型"}
          </span>
          <ChevronDown data-icon="inline-end" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side="top"
        align="start"
        sideOffset={8}
        collisionPadding={16}
        className="w-80 max-w-[calc(100vw-32px)] rounded-2xl bg-(--floating-surface) p-1.5 shadow-xl"
        aria-label="选择模型"
      >
        <Command label="搜索模型">
          <CommandInput aria-label="搜索模型" placeholder="搜索模型" />
          <CommandList className="max-h-80">
            <CommandEmpty>暂无匹配的可用模型</CommandEmpty>
            <CommandGroup>
              {models
                .filter((model) => model.current_version?.id)
                .map((model) => {
                  const unavailable =
                    model.status !== "active" ||
                    model.provider_status !== "active" ||
                    !model.credential_present;
                  return (
                    <CommandItem
                      key={model.current_version!.id}
                      value={model.current_version!.id}
                      keywords={[
                        model.display_name ?? "",
                        model.provider_name ?? "",
                        model.key ?? "",
                      ]}
                      disabled={unavailable}
                      onSelect={() => {
                        onSelect(model);
                        setOpen(false);
                      }}
                      className="gap-3 py-3"
                    >
                      <Box />
                      <span className="flex min-w-0 flex-1 flex-col gap-1">
                        <span className="truncate">{model.display_name}</span>
                        <span className="truncate text-xs text-muted-foreground">
                          {unavailable ? "当前不可用" : model.provider_name}
                        </span>
                      </span>
                      {model.current_version!.id === value ? (
                        <Check aria-label="当前模型" />
                      ) : null}
                    </CommandItem>
                  );
                })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

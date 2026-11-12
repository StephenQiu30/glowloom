"use client";

import type { ComponentProps, ReactNode } from "react";
import { cn } from "cn";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import { SelectTrigger } from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";

/** Shared presentation; draft, model and submission ownership stay with the caller. */
export function ComposerToolbar({
  className,
  ...props
}: ComponentProps<typeof Toolbar>) {
  return (
    <Toolbar
      role="group"
      data-align="block-end"
      data-composer-toolbar=""
      className={cn("composer-toolbar", className)}
      {...props}
    />
  );
}

export function ComposerToolbarActions({
  className,
  ...props
}: ComponentProps<typeof ToolbarGroup>) {
  return (
    <ToolbarGroup
      appearance="plain"
      className={cn("composer-toolbar-actions", className)}
      {...props}
    />
  );
}

export function ComposerToolbarButton({
  className,
  type = "button",
  variant = "ghost",
  size = "toolbar",
  ...props
}: ComponentProps<typeof Button>) {
  return (
    <Button
      type={type}
      variant={variant}
      size={size}
      className={cn("composer-control", className)}
      {...props}
    />
  );
}

export function ComposerSelectTrigger({
  className,
  ...props
}: ComponentProps<typeof SelectTrigger>) {
  return (
    <SelectTrigger
      variant="quiet"
      size="toolbar"
      className={cn("composer-control", className)}
      {...props}
    />
  );
}

export function ComposerPopoverContent({
  className,
  width = "parameters",
  ...props
}: ComponentProps<typeof PopoverContent> & {
  width?: "parameters" | "compact" | "quantity";
}) {
  return (
    <PopoverContent
      side="top"
      align="start"
      sideOffset={8}
      collisionPadding={16}
      data-composer-popover={width}
      data-canvas-no-zoom=""
      data-canvas-shortcuts-ignore=""
      className={cn("composer-popover", className)}
      {...props}
    />
  );
}

export function ComposerToolbarPopover({
  label,
  value,
  children,
  disabled,
  width,
  chevron = true,
}: {
  label: string;
  value: string;
  children: ReactNode;
  disabled?: boolean;
  width?: ComponentProps<typeof ComposerPopoverContent>["width"];
  chevron?: boolean;
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <ComposerToolbarButton aria-label={label} disabled={disabled}>
          <span className="min-w-0 truncate">{value}</span>
          {chevron && <ChevronDown data-icon="inline-end" />}
        </ComposerToolbarButton>
      </PopoverTrigger>
      <ComposerPopoverContent width={width} aria-label={label}>
        <PopoverTitle className="sr-only">{label}</PopoverTitle>
        {children}
      </ComposerPopoverContent>
    </Popover>
  );
}

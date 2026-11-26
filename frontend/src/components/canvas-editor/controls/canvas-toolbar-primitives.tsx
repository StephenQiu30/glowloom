import type { ComponentProps } from "react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { ToolbarGroup } from "@/components/ui/toolbar";

export function CanvasToolbarGroup({
  className,
  layout = "row",
  ...props
}: ComponentProps<typeof ToolbarGroup> & {
  layout?: "row" | "rail" | "grid";
}) {
  return (
    <ToolbarGroup
      className={cn("canvas-toolbar-group", className)}
      data-layout={layout}
      {...props}
    />
  );
}

export function CanvasToolbarDivider({ className }: { className?: string }) {
  return (
    <Separator
      orientation="vertical"
      className={cn("canvas-toolbar-divider", className)}
    />
  );
}

export function CanvasToolbarButton({
  asChild = false,
  className,
  iconOnly = true,
  variant = "ghost",
  ...props
}: Omit<ComponentProps<typeof Button>, "size"> & {
  iconOnly?: boolean;
}) {
  return (
    <Button
      asChild={asChild}
      type={asChild ? undefined : "button"}
      variant={variant}
      size={iconOnly ? "toolbar-icon" : "toolbar"}
      className={cn("canvas-toolbar-button", className)}
      {...props}
    />
  );
}

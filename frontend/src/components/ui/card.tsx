import * as React from "react";
import { cn } from "cn";

function Card({
  className,
  size = "default",
  variant = "default",
  ...props
}: React.ComponentProps<"div"> & {
  size?: "default" | "sm";
  variant?: "default" | "project" | "composer" | "folder" | "panel" | "danger";
}) {
  return (
    <div
      data-slot="card"
      data-size={size}
      data-variant={variant}
      className={cn(
        "group/card relative isolate flex flex-col gap-(--card-spacing) overflow-hidden rounded-lg bg-card py-(--card-spacing) text-sm text-card-foreground [--card-spacing:--spacing(4)] has-data-[slot=card-footer]:pb-0 has-[>img:first-child]:pt-0 data-[selected=true]:before:absolute data-[selected=true]:before:-inset-2 data-[selected=true]:before:-z-10 data-[selected=true]:before:rounded-lg data-[selected=true]:before:bg-surface-2 data-[size=sm]:[--card-spacing:--spacing(3)] data-[size=sm]:has-data-[slot=card-footer]:pb-0 data-[variant=composer]:rounded-3xl data-[variant=composer]:bg-surface-1 data-[variant=composer]:py-0 data-[variant=composer]:shadow-sm data-[variant=danger]:bg-destructive/10 data-[variant=folder]:flex-row data-[variant=folder]:items-center data-[variant=folder]:gap-3 data-[variant=folder]:p-3 data-[variant=panel]:bg-surface-1 data-[variant=panel]:[--card-spacing:--spacing(4)] data-[variant=project]:overflow-visible data-[variant=project]:bg-transparent data-[variant=project]:py-0 data-[variant=project]:[--card-spacing:0px] *:[img:first-child]:rounded-t-xl *:[img:last-child]:rounded-b-xl",
        className,
      )}
      {...props}
    />
  );
}

function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-header"
      className={cn(
        "group/card-header @container/card-header grid auto-rows-min items-start gap-1 rounded-t-xl px-(--card-spacing) has-data-[slot=card-action]:grid-cols-[1fr_auto] has-data-[slot=card-description]:grid-rows-[auto_auto] [.border-b]:pb-(--card-spacing)",
        className,
      )}
      {...props}
    />
  );
}

function CardTitle({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-title"
      className={cn(
        "font-heading text-sm leading-snug font-medium group-data-[size=sm]/card:text-sm group-data-[variant=project]/card:text-sm",
        className,
      )}
      {...props}
    />
  );
}

function CardDescription({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-description"
      className={cn(
        "text-xs text-muted-foreground group-data-[variant=danger]/card:text-destructive",
        className,
      )}
      {...props}
    />
  );
}

function CardContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-content"
      className={cn("px-(--card-spacing)", className)}
      {...props}
    />
  );
}

function CardFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-footer"
      className={cn(
        "flex items-center rounded-b-xl p-(--card-spacing)",
        className,
      )}
      {...props}
    />
  );
}

export {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardDescription,
  CardContent,
};

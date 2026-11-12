import type { ComponentProps } from "react";
import { cva } from "class-variance-authority";
import { cn } from "cn";
import { Scan } from "lucide-react";

const coverVariants = cva(
  "relative flex aspect-video w-full items-center justify-center overflow-hidden rounded-[var(--radius-panel)] text-workspace-muted-foreground",
  {
    variants: { variant: { default: "bg-muted", create: "bg-card" } },
    defaultVariants: { variant: "default" },
  },
);

export function AssetCardCover({
  children,
  variant = "default",
}: {
  children?: React.ReactNode;
  variant?: "default" | "create";
}) {
  return (
    <span data-asset-cover className={coverVariants({ variant })}>
      {children ?? <Scan aria-hidden="true" />}
    </span>
  );
}

export function AssetCardTitle({
  children,
  reserveAction = false,
}: {
  children: React.ReactNode;
  reserveAction?: boolean;
}) {
  return (
    <span
      className={cn(
        "resource-card-title mt-2 w-full truncate text-workspace-foreground",
        reserveAction && "pr-12 max-md:pr-14",
      )}
      title={typeof children === "string" ? children : undefined}
    >
      {children}
    </span>
  );
}

export function AssetCardMeta({
  reserveAction = false,
  ...props
}: ComponentProps<"span"> & { reserveAction?: boolean }) {
  return (
    <span
      {...props}
      className={cn(
        "resource-card-caption mt-2 w-full truncate text-workspace-subtle-foreground",
        reserveAction && "pr-12 max-md:pr-14",
      )}
    />
  );
}

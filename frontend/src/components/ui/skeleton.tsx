import { cn } from "cn";

function Skeleton({
  className,
  variant = "default",
  ...props
}: React.ComponentProps<"div"> & {
  variant?: "default" | "canvas-generation";
}) {
  return (
    <div
      data-slot="skeleton"
      data-variant={variant}
      className={cn(
        "animate-pulse rounded-md",
        variant === "canvas-generation" ? "bg-card" : "bg-muted",
        className,
      )}
      {...props}
    />
  );
}

export { Skeleton };

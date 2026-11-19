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
        "animate-[workspace-skeleton-pulse_1.6s_ease-in-out_infinite] rounded-md motion-reduce:animate-none",
        variant === "canvas-generation" ? "bg-card" : "bg-accent",
        className,
      )}
      {...props}
    />
  );
}

export { Skeleton };

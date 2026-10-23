import { BrandMark as ProductMark } from "@/components/brand/brand";
import { cn } from "@/lib/utils";
export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex size-8 shrink-0 items-center justify-center",
        className,
      )}
    >
      <ProductMark />
    </span>
  );
}

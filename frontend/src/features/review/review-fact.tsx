"use client";

import { cn } from "@/lib/class-names";

export function Fact({ label, mono = false, value }: { label: string; mono?: boolean; value: string }) {
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className={cn("mt-1 break-all", mono && "font-mono text-xs")}>{value}</p>
    </div>
  );
}

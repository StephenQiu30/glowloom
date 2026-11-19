import type { ReactNode } from "react";
import { Empty, EmptyDescription } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";

export function SidebarEmpty({ children }: { children: ReactNode }) {
  return (
    <Empty className="items-start px-2 py-1 text-start">
      <EmptyDescription className="text-xs/5">{children}</EmptyDescription>
    </Empty>
  );
}

export function SidebarHistoryLoading({ message }: { message: string }) {
  return (
    <li>
      <span role="status" className="sr-only">
        {message}
      </span>
      <div aria-hidden="true" className="flex flex-col gap-0.5">
        {["w-3/4", "w-2/3", "w-1/2"].map((width) => (
          <div key={width} className="flex h-9 items-center gap-2 px-2">
            <Skeleton className="size-5 shrink-0" />
            <Skeleton className={`h-3 max-w-32 ${width}`} />
          </div>
        ))}
      </div>
    </li>
  );
}

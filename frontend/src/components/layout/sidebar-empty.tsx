import type { ReactNode } from "react";
import { Empty, EmptyDescription } from "@/components/ui/empty";

export function SidebarEmpty({ children }: { children: ReactNode }) {
  return (
    <Empty className="items-start px-2 py-1 text-start">
      <EmptyDescription className="text-xs/5">{children}</EmptyDescription>
    </Empty>
  );
}

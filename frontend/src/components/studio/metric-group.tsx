import type { ReactNode } from "react";

import {
  Item,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from "@/components/ui/item";
import { cn } from "@/lib/class-names";

const columnClasses = {
  3: "xl:grid-cols-3",
  4: "xl:grid-cols-4",
  5: "xl:grid-cols-5",
} as const;

export type MetricItem = {
  label: ReactNode;
  value: ReactNode;
};

export function MetricGroup({
  className,
  columns = 5,
  items,
  label,
}: {
  className?: string;
  columns?: keyof typeof columnClasses;
  items: MetricItem[];
  label: string;
}) {
  return (
    <section aria-label={label} className={cn("bg-muted/45 px-2 py-1", className)}>
      <ItemGroup
        className={cn("grid gap-0 sm:grid-cols-2", columnClasses[columns])}
        role="list"
      >
        {items.map((item, index) => (
          <Item
            className="rounded-none border-0 px-4 py-4 sm:px-5"
            key={`${String(item.label)}:${index}`}
            role="listitem"
          >
            <ItemContent className="gap-2">
              <ItemDescription className="line-clamp-none text-xs font-medium tracking-wide">{item.label}</ItemDescription>
              <ItemTitle className="font-mono text-3xl font-semibold tracking-[-0.04em]">
                {item.value}
              </ItemTitle>
            </ItemContent>
          </Item>
        ))}
      </ItemGroup>
    </section>
  );
}

"use client";
import { Columns3 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuCheckboxItem,
} from "@/components/ui/dropdown-menu";
export function ColumnMenu({
  columns,
  visibility,
  onChange,
}: {
  columns: { id: string; label: string }[];
  visibility: Record<string, boolean>;
  onChange: (next: Record<string, boolean>) => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost">
          <Columns3 data-icon="inline-start" />
          显示列
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          {columns.map((c) => (
            <DropdownMenuCheckboxItem
              key={c.id}
              checked={visibility[c.id] !== false}
              onCheckedChange={(checked) =>
                onChange({ ...visibility, [c.id]: checked })
              }
              onSelect={(e) => e.preventDefault()}
            >
              {c.label}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

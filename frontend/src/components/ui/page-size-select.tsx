"use client";

import { useId } from "react";
import { Field, FieldLabel } from "@/components/ui/field";
import { PAGE_SIZE_OPTIONS as pageSizes } from "@/lib/pagination";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export type PageSize = (typeof pageSizes)[number];

export function PageSizeSelect({
  label = "每页数量",
  value,
  disabled,
  onChange,
}: {
  label?: string;
  value: number;
  disabled?: boolean;
  onChange: (size: PageSize) => void;
}) {
  const id = useId();
  return (
    <Field orientation="horizontal" className="w-auto gap-2">
      <FieldLabel htmlFor={id} className="max-sm:sr-only">
        每页
      </FieldLabel>
      <Select
        value={String(value)}
        disabled={disabled}
        onValueChange={(value) => {
          const size = pageSizes.find((size) => String(size) === value);
          if (size) onChange(size);
        }}
      >
        <SelectTrigger
          id={id}
          aria-label={label}
          className="w-20 text-sm data-[size=default]:h-9 max-md:data-[size=default]:h-11"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          <SelectGroup>
            {pageSizes.map((size) => (
              <SelectItem key={size} value={String(size)}>
                {size}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  );
}

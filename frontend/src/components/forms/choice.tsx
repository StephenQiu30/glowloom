"use client";

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function Choice({
  value,
  onChange,
  options,
  label,
  className,
  prefix,
  size,
}: {
  value: string;
  onChange: (value: string) => void;
  options: readonly string[];
  label: string;
  className?: string;
  prefix?: string;
  size?: "sm" | "default";
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger aria-label={label} className={className} size={size}>
        {prefix ? <span>{prefix}</span> : null}
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

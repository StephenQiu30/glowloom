"use client";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
export function LibrarySelect({
  id,
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  id?: string;
  label: string;
  value: string;
  options: readonly { value: string; label: string; disabled?: boolean }[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger id={id} aria-label={label} className="w-full">
        <SelectValue placeholder={label} />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {options.map((option) => (
            <SelectItem
              key={option.value}
              value={option.value}
              disabled={option.disabled}
            >
              {option.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

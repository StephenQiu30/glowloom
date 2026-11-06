"use client";

import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";

type FilterOption = { value: string; label: string };

export type AdminTableFilterConfig<Key extends string> = {
  name: Key;
  label?: string;
} & (
  | { type: "text"; placeholder?: string }
  | { type: "select"; options: readonly FilterOption[] }
);

export type AdminTableFilterValues<Key extends string> = Partial<
  Record<Key, string>
>;

export function AdminTableFilters<Key extends string>({
  label,
  fields,
  disabled,
  onApply,
}: {
  label: string;
  fields: readonly (AdminTableFilterConfig<Key> & { label: string })[];
  disabled?: boolean;
  onApply: (values: AdminTableFilterValues<Key>) => void;
}) {
  const id = useId();
  const [draft, setDraft] = useState<AdminTableFilterValues<Key>>({});
  return (
    <form
      aria-label={label}
      className="grid grid-cols-2 items-end gap-3 sm:flex sm:flex-wrap"
      onSubmit={(event) => {
        event.preventDefault();
        if (disabled) return;
        const values: AdminTableFilterValues<Key> = {};
        for (const field of fields) {
          const value = draft[field.name]?.trim();
          if (value) values[field.name] = value;
        }
        onApply(values);
      }}
    >
      {fields.map((field) => {
        const fieldId = `${id}-${field.name}`;
        const props = {
          id: fieldId,
          name: field.name,
          disabled,
          value: draft[field.name] ?? "",
          onChange: (
            event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>,
          ) => {
            const value = event.target.value;
            setDraft((previous) => ({ ...previous, [field.name]: value }));
          },
        };
        return (
          <Field key={field.name} className="w-full min-w-0 gap-1.5 sm:w-40">
            <FieldLabel htmlFor={fieldId}>{field.label}</FieldLabel>
            {field.type === "text" ? (
              <Input
                {...props}
                placeholder={field.placeholder}
                maxLength={100}
              />
            ) : (
              <NativeSelect {...props} className="w-full">
                <NativeSelectOption value="">全部</NativeSelectOption>
                {field.options.map((option) => (
                  <NativeSelectOption key={option.value} value={option.value}>
                    {option.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            )}
          </Field>
        );
      })}
      <div className="col-span-2 flex gap-2">
        <Button
          type="submit"
          size="toolbar"
          variant="outline"
          disabled={disabled}
        >
          筛选
        </Button>
        <Button
          size="toolbar"
          type="button"
          variant="ghost"
          disabled={disabled}
          onClick={() => {
            setDraft({});
            onApply({});
          }}
        >
          重置
        </Button>
      </div>
    </form>
  );
}

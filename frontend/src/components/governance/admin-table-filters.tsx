"use client";

import { useId, useState } from "react";
import { SlidersHorizontal } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
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
  const [open, setOpen] = useState(false);
  const [appliedCount, setAppliedCount] = useState(0);
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="admin-table-filters"
    >
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="toolbar" disabled={disabled}>
          <SlidersHorizontal data-icon="inline-start" />
          筛选条件{appliedCount ? ` · ${appliedCount}` : ""}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent forceMount className="data-[state=closed]:hidden">
        <form
          aria-label={label}
          className="flex flex-col gap-3 pt-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (disabled) return;
            const values: AdminTableFilterValues<Key> = {};
            for (const field of fields) {
              const value = draft[field.name]?.trim();
              if (value) values[field.name] = value;
            }
            onApply(values);
            setAppliedCount(Object.keys(values).length);
            setOpen(false);
          }}
        >
          <FieldGroup className="grid grid-cols-1 items-end gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {fields.map((field) => {
              const fieldId = `${id}-${field.name}`;
              const props = {
                id: fieldId,
                name: field.name,
                disabled,
                value: draft[field.name] ?? "",
                onChange: (
                  event: React.ChangeEvent<
                    HTMLInputElement | HTMLSelectElement
                  >,
                ) => {
                  const value = event.target.value;
                  setDraft((previous) => ({
                    ...previous,
                    [field.name]: value,
                  }));
                },
              };
              return (
                <Field
                  key={field.name}
                  data-disabled={disabled}
                  className="min-w-0"
                >
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
                        <NativeSelectOption
                          key={option.value}
                          value={option.value}
                        >
                          {option.label}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  )}
                </Field>
              );
            })}
          </FieldGroup>
          <div className="flex gap-2">
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
                setAppliedCount(0);
                onApply({});
              }}
            >
              重置筛选
            </Button>
          </div>
        </form>
      </CollapsibleContent>
    </Collapsible>
  );
}

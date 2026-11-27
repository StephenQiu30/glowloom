"use client";

import { useId, useState, type ComponentProps, type ReactNode } from "react";
import { ChevronDown, Save, type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

import { arkEmbedding, type GatewayPrice } from "@/lib/ai-service-admin";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

export type RunMutation = (
  action: () => Promise<unknown>,
  success: string,
) => Promise<boolean>;

export const uuidPattern =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";

export function InputField({
  label,
  description,
  value,
  required = true,
  ...props
}: Omit<ComponentProps<"input">, "value" | "id"> & {
  label: string;
  description?: string;
  value?: string | number;
}) {
  const id = useId();
  return (
    <Field data-disabled={props.disabled || undefined}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        defaultValue={value}
        required={required}
        aria-describedby={description ? `${id}-hint` : undefined}
        autoComplete={props.type === "password" ? "new-password" : "off"}
        className="max-md:min-h-11 max-md:text-base"
        {...props}
      />
      {description && (
        <FieldDescription id={`${id}-hint`}>{description}</FieldDescription>
      )}
    </Field>
  );
}

export function SelectField({
  label,
  options,
  description,
  ...props
}: Omit<ComponentProps<typeof NativeSelect>, "children" | "id"> & {
  label: string;
  options: readonly (readonly [string, string])[];
  description?: string;
}) {
  const id = useId();
  return (
    <Field data-disabled={props.disabled || undefined} className="min-w-0">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <NativeSelect
        id={id}
        className="w-full min-w-0 max-md:min-h-11 max-md:[&>select]:text-base"
        aria-describedby={description ? `${id}-hint` : undefined}
        {...props}
      >
        {options.map(([key, text]) => (
          <NativeSelectOption key={key} value={key}>
            {text}
          </NativeSelectOption>
        ))}
      </NativeSelect>
      {description && (
        <FieldDescription id={`${id}-hint`}>{description}</FieldDescription>
      )}
    </Field>
  );
}

export function SetupSelectField({
  label,
  options,
  placeholder,
  description,
  displayValue,
  ...props
}: ComponentProps<typeof Select> & {
  label: string;
  options: readonly (readonly [
    value: string,
    title: string,
    detail?: { description: string; icon: ReactNode },
  ])[];
  placeholder?: string;
  description?: string;
  displayValue?: string;
}) {
  const id = useId();
  return (
    <Field data-disabled={props.disabled || undefined} className="min-w-0">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select {...props}>
        <SelectTrigger
          id={id}
          aria-describedby={description ? `${id}-hint` : undefined}
          className="w-full max-md:text-base"
        >
          <SelectValue placeholder={placeholder}>{displayValue}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map(([value, title, detail]) => (
              <SelectItem
                key={value}
                value={value}
                textValue={detail ? `${title} ${detail.description}` : title}
              >
                {detail ? (
                  <span className="flex min-w-0 items-center gap-2.5">
                    <span className="flex shrink-0 items-center justify-center rounded-md bg-muted p-1.5">
                      {detail.icon}
                    </span>
                    <span className="flex min-w-0 flex-col">
                      <span className="font-medium">{title}</span>
                      <span className="text-xs text-muted-foreground">
                        {detail.description}
                      </span>
                    </span>
                  </span>
                ) : (
                  title
                )}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {description && (
        <FieldDescription id={`${id}-hint`}>{description}</FieldDescription>
      )}
    </Field>
  );
}

export function TextField({
  label,
  ...props
}: Omit<ComponentProps<"textarea">, "id"> & { label: string }) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Textarea id={id} className="max-md:text-base" {...props} />
    </Field>
  );
}

export function CheckField({
  label,
  ...props
}: ComponentProps<typeof Checkbox> & { label: string }) {
  const id = useId();
  return (
    <Field orientation="horizontal" className="max-md:min-h-11">
      <Checkbox id={id} {...props} />
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
    </Field>
  );
}

export function FormDetails({
  label,
  children,
  keepMounted = true,
}: {
  label: string;
  children: ReactNode;
  keepMounted?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      onInvalidCapture={(event) => {
        if (open || !(event.target instanceof HTMLElement)) return;
        const target = event.target;
        setOpen(true);
        requestAnimationFrame(() => target.focus());
      }}
    >
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="w-full justify-between max-md:min-h-11"
        >
          {label}
          <ChevronDown
            data-icon="inline-end"
            className={open ? "rotate-180" : undefined}
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent
        forceMount={keepMounted ? true : undefined}
        className="pt-3 data-[state=closed]:hidden"
      >
        {children}
      </CollapsibleContent>
    </Collapsible>
  );
}

export function SubmitButton({
  busy,
  label,
  icon = Save,
  disabled,
}: {
  busy: boolean;
  label: string;
  icon?: LucideIcon;
  disabled?: boolean;
}) {
  const Icon = icon;
  return (
    <Button
      type="submit"
      className="max-sm:min-h-11 max-sm:w-full"
      disabled={busy || disabled}
      aria-busy={busy}
    >
      {busy ? (
        <Spinner aria-hidden="true" data-icon="inline-start" />
      ) : (
        <Icon data-icon="inline-start" />
      )}
      {label}
    </Button>
  );
}

export function text(form: FormData, key: string) {
  return String(form.get(key) ?? "").trim();
}

export function num(form: FormData, key: string) {
  return Number(text(form, key));
}

export function micros(form: FormData, key: string) {
  const raw = text(form, key) || "0";
  if (!/^\d+(?:\.\d{1,6})?$/.test(raw))
    throw new Error("金额须为非负数字，最多保留六位小数。");
  const [whole, fraction = ""] = raw.split(".");
  const amount =
    BigInt(whole) * BigInt(1000000) + BigInt(fraction.padEnd(6, "0"));
  if (amount > BigInt(Number.MAX_SAFE_INTEGER))
    throw new Error("金额超出可用范围。");
  return Number(amount);
}

export function money(value: number, currency = "CNY") {
  const whole = Math.trunc(value / 1000000);
  const fraction = String(value % 1000000)
    .padStart(6, "0")
    .replace(/0+$/, "")
    .padEnd(2, "0");
  if (!/^[A-Z]{3}$/.test(currency))
    return `${whole.toLocaleString("zh-CN")}.${fraction} ${currency}`;
  const format = new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return format
    .formatToParts(whole)
    .map((part) => (part.type === "fraction" ? fraction : part.value))
    .join("");
}

export function priceSummary(price: GatewayPrice, protocol: string) {
  if (price.explicit_free === true) return "免费";
  if (protocol === arkEmbedding.protocol)
    return `输入 ${money(price.input, price.currency)} / 百万 token`;
  if (protocol === "media" && price.output > 0 && price.request === 0)
    return `${money(price.output, price.currency)} / 百万视频 Token`;
  return protocol === "media"
    ? `${money(price.request, price.currency)} / 次`
    : `输入 ${money(price.input, price.currency)} · 输出 ${money(price.output, price.currency)} / 百万 token`;
}

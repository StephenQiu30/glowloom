"use client";
import { useEffect, useId, useRef, type ComponentProps } from "react";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

export function AccountField({
  label,
  description,
  ...props
}: ComponentProps<typeof Input> & { label: string; description?: string }) {
  const id = useId();
  return (
    <Field data-disabled={props.disabled} data-invalid={props["aria-invalid"]}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        aria-describedby={description ? `${id}-description` : undefined}
        {...props}
      />
      {description && (
        <FieldDescription id={`${id}-description`}>
          {description}
        </FieldDescription>
      )}
    </Field>
  );
}
export function AccountError({ error }: { error?: Error }) {
  const reported = useRef<Error | undefined>(undefined);
  useEffect(() => {
    if (!error || reported.current === error) return;
    reported.current = error;
    notifyError(error, "账户操作未完成");
  }, [error]);
  return null;
}

export function AccountForm(props: ComponentProps<"form">) {
  const reporting = useRef(false);
  return (
    <form
      {...props}
      onInputCapture={(event) => {
        const input = event.target as HTMLElement;
        input.removeAttribute("aria-invalid");
        input.closest('[data-slot="field"]')?.removeAttribute("data-invalid");
        props.onInputCapture?.(event);
      }}
      onInvalidCapture={(event) => {
        event.preventDefault();
        const input = event.target as HTMLInputElement;
        input.setAttribute("aria-invalid", "true");
        input
          .closest('[data-slot="field"]')
          ?.setAttribute("data-invalid", "true");
        if (reporting.current) return;
        reporting.current = true;
        const label = input.labels?.[0]?.textContent || "此项";
        const message = input.validity.valueMissing
          ? `请填写${label}`
          : `请检查${label}的格式与长度`;
        setTimeout(() => {
          notifyError(new Error(message), "请检查输入");
          input.focus();
          reporting.current = false;
        }, 0);
      }}
    />
  );
}

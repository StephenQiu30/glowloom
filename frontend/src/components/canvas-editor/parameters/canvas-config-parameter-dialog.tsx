"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

/** 显式编辑原生参数名与 JSON 值，不生成占位参数。 */
export function CanvasConfigParameterDialog({
  values,
  parameterKey,
  onChange,
  disabled,
}: {
  values: Record<string, unknown>;
  parameterKey?: string;
  onChange: (values: Record<string, unknown>) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) {
          setName(parameterKey ?? "");
          setValue(
            parameterKey ? JSON.stringify(values[parameterKey], null, 2) : "",
          );
          setError("");
        }
        setOpen(next);
      }}
    >
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm" disabled={disabled}>
          {parameterKey ? `编辑 ${parameterKey}` : "添加参数"}
        </Button>
      </DialogTrigger>
      <DialogContent className="dialog-settings">
        <DialogHeader>
          <DialogTitle>
            {parameterKey ? "编辑共享参数" : "添加共享参数"}
          </DialogTitle>
          <DialogDescription>
            填写模型支持的参数名。参数值使用 JSON：数字如 1，文字如
            &quot;16:9&quot;，开关如 true。
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            const key = name.trim();
            if (!key || (key !== parameterKey && Object.hasOwn(values, key))) {
              setError(!key ? "请输入参数名" : "参数名已存在");
              return;
            }
            try {
              const parsed: unknown = JSON.parse(value);
              const next = { ...values };
              if (parameterKey && parameterKey !== key)
                delete next[parameterKey];
              onChange({ ...next, [key]: parsed });
              setOpen(false);
            } catch {
              setError("请输入有效的 JSON 参数值，文字需要使用双引号");
            }
          }}
        >
          <FieldGroup>
            <Field data-invalid={!!error}>
              <FieldLabel htmlFor="config-parameter-name">参数名</FieldLabel>
              <Input
                id="config-parameter-name"
                aria-invalid={!!error}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field data-invalid={!!error}>
              <FieldLabel htmlFor="config-parameter-value">
                参数值（JSON）
              </FieldLabel>
              <Textarea
                id="config-parameter-value"
                aria-invalid={!!error}
                value={value}
                onChange={(event) => setValue(event.target.value)}
              />
            </Field>
            {error && <FieldError>{error}</FieldError>}
            <DialogFooter>
              {parameterKey && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    const next = { ...values };
                    delete next[parameterKey];
                    onChange(next);
                    setOpen(false);
                  }}
                >
                  移除参数
                </Button>
              )}
              <Button type="submit">保存参数</Button>
            </DialogFooter>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  );
}

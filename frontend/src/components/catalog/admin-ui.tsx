"use client";

import { useState, type ReactNode } from "react";
import { ApiError } from "@/lib/request";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { adminError } from "./admin-queries";

export function AdminField({
  id,
  label,
  error,
  children,
}: {
  id: string;
  label: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <Field data-invalid={!!error}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {children}
      {error && <FieldError>{error}</FieldError>}
    </Field>
  );
}
export function AdminSelect({
  id,
  value,
  onChange,
  options,
  disabled = false,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
  disabled?: boolean;
}) {
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger id={id} className="w-full">
        <SelectValue placeholder="请选择" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {options.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
export function AdminFailure({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  const forbidden = error instanceof ApiError && error.status === 403;
  return (
    <Alert role="alert">
      <AlertTitle>
        {forbidden ? "需要管理员权限" : "配置暂时无法读取"}
      </AlertTitle>
      <AlertDescription>
        <p>
          {forbidden
            ? "当前工作区可查看项目模型。渠道、凭据和模型版本的管理请求被服务端拒绝。"
            : adminError(error)}
        </p>
        {retry && (
          <Button variant="outline" className="mt-3" onClick={retry}>
            重新读取
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}
export function AdminMessage({
  message,
  error = false,
}: {
  message: string;
  error?: boolean;
}) {
  return message ? (
    <p
      role={error ? "alert" : "status"}
      className={
        error ? "text-sm text-destructive" : "text-sm text-muted-foreground"
      }
    >
      {message}
    </p>
  ) : null;
}
export function AdminDialog({
  title,
  description,
  children,
  disabled = false,
}: {
  title: string;
  description: string;
  children: (close: () => void) => ReactNode;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" disabled={disabled}>
          {title}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {open && children(() => setOpen(false))}
      </DialogContent>
    </Dialog>
  );
}
export function formatAdminTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

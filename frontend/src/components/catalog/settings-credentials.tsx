"use client";

import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  disableAdminCredential,
  setAdminCredential,
  testAdminCredential,
} from "@/gen/api/settings";
import { ApiError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  adminError,
  adminWriteOptions,
  credentialTestAcceptedSchema,
  parseAdmin,
  type ProviderDetail,
} from "./admin-queries";
import {
  AdminDialog,
  AdminField,
  AdminMessage,
  formatAdminTime,
} from "./admin-ui";

const testNames = {
  ok: "验证通过",
  auth_failed: "认证失败",
  unreachable: "无法连接",
  timeout: "测试超时",
  unsupported: "适配器不支持此测试",
};
const savedCredential = z.object({
  id: z.string().uuid(),
  provider_id: z.string().uuid(),
  last4: z.string().length(4),
  status: z.enum(["active", "disabled"]),
});

export function SettingsCredentials({
  detail,
  refresh,
}: {
  detail: ProviderDetail;
  refresh: () => void;
}) {
  const { provider, credential, credential_schema: fields } = detail;
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  const schema = z.object({
    label: z.string().trim().max(100),
    secret: z
      .record(z.string(), z.string().max(4096))
      .superRefine((values, ctx) => {
        for (const field of fields) {
          if (field.required && !values[field.name])
            ctx.addIssue({
              code: "custom",
              path: [field.name],
              message: "此字段必填",
            });
        }
      }),
  });
  const emptySecret = Object.fromEntries(
    fields.map((field) => [field.name, ""]),
  );
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { label: "", secret: emptySecret },
  });
  const supported =
    fields.length > 0 &&
    fields.every((field) =>
      ["password", "text", "string"].includes(field.type),
    );
  async function run(action: (signal: AbortSignal) => Promise<string>) {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setMessage("");
    setFailed(false);
    const controller = new AbortController();
    pending.current = controller;
    try {
      setMessage(await action(controller.signal));
    } catch (error) {
      if (!controller.signal.aborted) {
        setMessage(adminError(error));
        setFailed(true);
      }
    } finally {
      form.reset({ label: "", secret: emptySecret });
      lock.current = false;
      setBusy(false);
      pending.current = null;
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>渠道凭据</CardTitle>
        <CardDescription>
          提交后输入内容会清空。已保存凭据仅显示名称、尾号和测试结果。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {credential ? (
          <div className="space-y-3 rounded-lg border p-4">
            <p>
              {credential.label} · 尾号 {credential.last4}
            </p>
            <p className="text-sm text-muted-foreground">
              {credential.last_test_result
                ? testNames[credential.last_test_result]
                : "尚无测试结果"}
              {credential.last_tested_at &&
                ` · ${formatAdminTime(credential.last_tested_at)}`}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={busy || provider.status !== "active"}
                onClick={() =>
                  void run(async (signal) => {
                    const result = parseAdmin(
                      credentialTestAcceptedSchema,
                      await testAdminCredential(
                        { id: provider.id, credential_id: credential.id },
                        { ...adminWriteOptions(), signal },
                      ),
                    );
                    if (
                      result.provider_id !== provider.id ||
                      result.credential_id !== credential.id
                    )
                      throw new ApiError(502, "invalid_response");
                    return "测试已受理，尚未取得验证结果。请重新读取渠道详情查看实际结果。";
                  })
                }
              >
                测试连接
              </Button>
              <Button variant="ghost" disabled={busy} onClick={refresh}>
                读取测试结果
              </Button>
              <AdminDialog
                title="禁用当前凭据"
                description={`确认禁用 ${credential.label}（尾号 ${credential.last4}）。禁用后将无法使用此凭据提交新任务。`}
                disabled={busy}
              >
                {(close) => (
                  <Button
                    variant="destructive"
                    disabled={busy}
                    onClick={() =>
                      void run(async (signal) => {
                        const result = parseAdmin(
                          savedCredential,
                          await disableAdminCredential(
                            { id: provider.id, credential_id: credential.id },
                            { ...adminWriteOptions(), signal },
                          ),
                        );
                        if (
                          result.id !== credential.id ||
                          result.provider_id !== provider.id ||
                          result.status !== "disabled"
                        )
                          throw new ApiError(502, "invalid_response");
                        refresh();
                        close();
                        return "凭据已禁用。";
                      })
                    }
                  >
                    确认禁用
                  </Button>
                )}
              </AdminDialog>
            </div>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            此渠道尚未配置有效凭据。
          </p>
        )}
        <form
          autoComplete="off"
          onSubmit={(event) =>
            void form.handleSubmit((values) =>
              run(async (signal) => {
                const result = parseAdmin(
                  savedCredential,
                  await setAdminCredential(
                    { id: provider.id },
                    { label: values.label || undefined, secret: values.secret },
                    { ...adminWriteOptions(), signal },
                  ),
                );
                if (
                  result.provider_id !== provider.id ||
                  result.status !== "active"
                )
                  throw new ApiError(502, "invalid_response");
                refresh();
                return "新凭据已保存，等待连接测试。";
              }),
            )(event)
          }
          className="space-y-4"
        >
          <FieldGroup>
            <AdminField
              id="credential-label"
              label="新凭据名称（可选）"
              error={form.formState.errors.label?.message}
            >
              <Input
                id="credential-label"
                {...form.register("label")}
                disabled={busy}
              />
            </AdminField>
            {fields.map((field) => (
              <AdminField
                key={field.name}
                id={`credential-${field.name}`}
                label={field.name}
                error={form.formState.errors.secret?.[field.name]?.message}
              >
                <Input
                  id={`credential-${field.name}`}
                  type="password"
                  autoComplete="new-password"
                  spellCheck={false}
                  {...form.register(`secret.${field.name}`)}
                  disabled={busy || !supported}
                  aria-invalid={!!form.formState.errors.secret?.[field.name]}
                />
              </AdminField>
            ))}
          </FieldGroup>
          {!supported && <p role="alert">此凭据字段类型尚未支持，无法保存。</p>}
          <Button type="submit" disabled={busy || !supported}>
            保存新凭据
          </Button>
        </form>
        <AdminMessage message={message} error={failed} />
      </CardContent>
    </Card>
  );
}

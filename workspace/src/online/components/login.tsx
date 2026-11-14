"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldDescription } from "@/components/ui/field";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { api } from "./api";
export function Login() {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <form
      className="form-stack"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setError("");
        try {
          await api("/session", { token });
          const next = new URL(location.href).searchParams.get("next") ?? "/";
          const url = new URL(next, location.origin);
          location.assign(
            url.origin === location.origin ? url.pathname + url.search : "/",
          );
        } catch (error) {
          setError(error instanceof Error ? error.message : "登录失败");
        } finally {
          setBusy(false);
        }
      }}
    >
      <Field>
        <FieldLabel htmlFor="access-token">本机访问令牌</FieldLabel>
        <Input
          id="access-token"
          type="password"
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          required
        />
        <FieldDescription>
          启动时生成的令牌保存在本机私有配置中；它不属于产品账号密码。
        </FieldDescription>
      </Field>
      {error && (
        <Alert>
          <AlertDescription role="alert">{error}</AlertDescription>
        </Alert>
      )}
      <Button type="submit" disabled={busy || !token}>
        {busy ? "正在验证…" : "进入工作空间"}
      </Button>
    </form>
  );
}

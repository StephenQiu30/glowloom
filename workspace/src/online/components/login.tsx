'use client';
import { useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
  FieldError,
} from '@/components/ui/field';
import { Spinner } from '@/components/ui/spinner';
import { api } from './api';

/** Keeps authentication outside the private knowledge shell. */
export function Login() {
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const input = useRef<HTMLInputElement>(null);
  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        setBusy(true);
        setError('');
        try {
          await api('/session', { token });
          const next = new URL(location.href).searchParams.get('next') ?? '/';
          const url = new URL(next, location.origin);
          location.assign(
            url.origin === location.origin ? url.pathname + url.search : '/',
          );
        } catch (error) {
          setError(error instanceof Error ? error.message : '登录失败');
          input.current?.focus();
        } finally {
          setBusy(false);
        }
      }}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(error)}>
          <FieldLabel htmlFor="access-token">本机访问令牌</FieldLabel>
          <Input
            ref={input}
            id="access-token"
            name="access-token"
            type="password"
            autoComplete="current-password"
            value={token}
            onChange={(event) => setToken(event.target.value)}
            required
            aria-invalid={Boolean(error)}
            aria-describedby={
              error ? 'token-description token-error' : 'token-description'
            }
          />
          <FieldDescription id="token-description">
            令牌保存在本机私有配置中，与产品账号密码分开管理。
          </FieldDescription>
          {error && (
            <FieldError id="token-error" role="alert">
              {error}
            </FieldError>
          )}
        </Field>
        <Button type="submit" disabled={busy}>
          {busy && <Spinner data-icon="inline-start" aria-label="正在验证" />}
          进入工作空间
        </Button>
      </FieldGroup>
    </form>
  );
}

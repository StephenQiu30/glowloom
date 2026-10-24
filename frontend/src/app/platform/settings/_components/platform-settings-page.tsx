"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";
import { useEffect, useState } from "react";
import { platformSettings, updatePlatformSettings } from "@/api/identity";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { Button } from "@/components/ui/button";
export function PlatformSettingsPage() {
  const [settings, setSettings] = useState<API.PlatformSettings | null>(null);
  const [registration, setRegistration] = useState(false);
  const [creation, setCreation] = useState(false);
  const [reload, setReload] = useState(0);
  const [error, setError] = useState("");
  const [action, setAction] = useState<ConfirmAction | null>(null);
  useEffect(() => {
    const c = new AbortController();
    platformSettings({ signal: c.signal })
      .then((s) => {
        if (!c.signal.aborted) {
          setSettings(s);
          setRegistration(Boolean(s.registration_enabled));
          setCreation(Boolean(s.team_creation_enabled));
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(readErrorMessage(e));
      });
    return () => c.abort();
  }, [reload]);
  return (
    <div className="mx-auto grid w-full max-w-2xl gap-4 p-4">
      <h1 className="text-2xl">平台设置</h1>
      {error ? (
        <ContentState
          state="error"
          title="平台设置暂时无法读取"
          description={error}
          actions={
            <Button
              onClick={() => {
                setError("");
                setSettings(null);
                setReload((n) => n + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : null}
      {settings && !error ? (
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            setAction({
              title: "保存平台设置",
              effect:
                "修改公开注册与团队创建资格。注册仍须满足邮箱验证和生效条款条件。",
              reason: true,
              run: (reason, signal) =>
                updatePlatformSettings(
                  {
                    expected_revision: settings.revision,
                    reason,
                    registration_enabled: registration,
                    team_creation_enabled: creation,
                  },
                  { signal },
                ),
            });
          }}
        >
          <FieldLabel className="flex gap-2">
            <Checkbox
              checked={registration}
              onCheckedChange={(value) => setRegistration(value === true)}
            />
            允许公开注册
          </FieldLabel>
          <FieldLabel className="flex gap-2">
            <Checkbox
              checked={creation}
              onCheckedChange={(value) => setCreation(value === true)}
            />
            允许创建团队
          </FieldLabel>
          <Button>保存平台设置</Button>
        </form>
      ) : !error ? (
        <ContentState state="loading" title="正在读取平台设置" />
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={() => setReload((n) => n + 1)}
      />
    </div>
  );
}

"use client";

import { useState } from "react";
import { updateProviderLogo } from "@/api/settings";
import { useSession } from "@/components/auth/session";
import { useCatalogMutation } from "@/components/catalog/mutation";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { ProviderLogoPicker } from "@/components/providers/provider-logo-picker";
import { Button } from "@/components/ui/button";

type LogoInput = Parameters<typeof updateProviderLogo>[1];
// Unknown writes retain their original nonsecret body for reopening in this session.
const pendingChanges = new Map<string, Omit<LogoInput, "request_id">>();
export function ConnectionLogoEditor({
  provider,
  disabled,
  onSaved,
}: {
  provider: API.CreatedProvider;
  disabled?: boolean;
  onSaved: () => void;
}) {
  const session = useSession().session_id;
  const scope = `${session}:${provider.id}`;
  const [open, setOpen] = useState(false);
  const [logo, setLogo] = useState<string | undefined>(
    () => pendingChanges.get(scope)?.logo_asset_id ?? provider.logo_asset_id,
  );
  const [uploading, setUploading] = useState(false);
  const mutation = useCatalogMutation(
    () => {
      pendingChanges.delete(scope);
      setOpen(false);
      onSaved();
    },
    `provider.logo.${provider.id}`,
    (result, target) => {
      const attempt = pendingChanges.get(scope);
      return (
        typeof result === "object" &&
        result !== null &&
        "id" in result &&
        result.id === target &&
        "revision" in result &&
        typeof result.revision === "number" &&
        (!attempt ||
          (("logo_asset_id" in result ? result.logo_asset_id : null) ??
            null) === (attempt.logo_asset_id ?? null))
      );
    },
  );
  const locked = disabled || mutation.busy || !!mutation.pending;
  return (
    <div className="flex flex-col gap-3 max-md:[&_button]:min-h-11">
      <div className="flex items-center gap-3">
        <ProviderLogo
          adapterKey={provider.adapter_key}
          logoAssetID={provider.logo_asset_id}
          iconOnly
        />
        {!open ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled || mutation.busy}
            onClick={() => {
              setLogo(
                pendingChanges.has(scope)
                  ? (pendingChanges.get(scope)?.logo_asset_id ?? undefined)
                  : provider.logo_asset_id,
              );
              setOpen(true);
            }}
          >
            编辑 Logo
          </Button>
        ) : null}
      </div>
      {mutation.message || mutation.pending ? (
        <Alert role="status">
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            {mutation.pending ? "Logo 保存待确认。" : mutation.message}
            {mutation.pending ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={mutation.busy}
                onClick={() => void mutation.recover()}
              >
                查询回执
              </Button>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}
      {open ? (
        <form
          className="flex flex-col gap-3"
          onSubmit={async (event) => {
            event.preventDefault();
            if (!provider.id || disabled || uploading || mutation.busy) return;
            if (mutation.pending && !pendingChanges.has(scope)) {
              await mutation.recover();
              return;
            }
            const input = mutation.pending
              ? pendingChanges.get(scope)!
              : {
                  expected_revision: provider.revision,
                  logo_asset_id: logo ?? null,
                };
            pendingChanges.set(scope, input);
            await mutation.run("provider.logo.set", provider.id, (request_id) =>
              updateProviderLogo(
                { id: provider.id! },
                { ...input, request_id },
              ),
            );
          }}
        >
          <ProviderLogoPicker
            adapterKey={provider.adapter_key}
            value={logo}
            onChange={setLogo}
            disabled={locked}
            onUploadingChange={setUploading}
          />
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={uploading || mutation.busy}
              onClick={() => {
                setLogo(provider.logo_asset_id);
                setOpen(false);
                if (!mutation.pending) pendingChanges.delete(scope);
              }}
            >
              取消
            </Button>
            <Button
              disabled={
                disabled ||
                uploading ||
                mutation.busy ||
                (!mutation.pending &&
                  (logo ?? null) === (provider.logo_asset_id ?? null))
              }
            >
              {mutation.busy
                ? "正在保存…"
                : mutation.pending
                  ? "核对并保存 Logo"
                  : "保存 Logo"}
            </Button>
          </div>
        </form>
      ) : null}
    </div>
  );
}

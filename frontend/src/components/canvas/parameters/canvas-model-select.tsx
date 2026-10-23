"use client";

import { useEffect, useId, useRef, useState } from "react";
import useSWR from "swr";
import { RefreshCw } from "lucide-react";
import * as models from "@/components/canvas/runtime/api/generationBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import { modelOptionLabel } from "@/components/canvas/runtime/components/generation/model/model-option";

type Selection = NonNullable<
  CanvasContract.CanvasNodeParametersDTO["modelSelection"]
>;

/** Directory reads only affect the choices; saved bindings remain editable offline. */
export function CanvasModelSelect({
  type,
  selected,
  onSelect,
}: {
  type: string;
  selected?: Selection;
  onSelect: (selection: Selection) => void;
}) {
  const statusId = useId();
  const { account, ready, signingOut, refresh } = useAccount();
  const [refreshing, setRefreshing] = useState(false);
  const scope = accountScope(account);
  const identity = useRef<string | null>(scope);
  useEffect(() => {
    identity.current = scope;
    return () => {
      identity.current = null;
    };
  }, [scope]);
  const enabled = ready && !!account && !signingOut;
  const catalog = useSWR(
    enabled ? ["canvas-node-models", scope, type] : null,
    async () =>
      unwrap(
        await (type === "image"
          ? models.listImageGenerationModels()
          : type === "video"
            ? models.listVideoGenerationModels()
            : models.listTextGenerationModels()),
      ),
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  const denied =
    catalog.error instanceof ApiError && catalog.error.code === 40301;
  useErrorNotification(denied ? undefined : catalog.error, "模型读取失败");
  const rows = catalog.error
    ? []
    : (catalog.data ?? []).filter(
        (row) => row.connectionId != null && row.modelId,
      );
  const first = rows[0];
  useEffect(() => {
    if (enabled && !selected && first?.connectionId != null && first.modelId)
      onSelect({ connectionId: first.connectionId, modelId: first.modelId });
  }, [enabled, selected, first?.connectionId, first?.modelId, onSelect]);
  const value = selected ? `${selected.connectionId}:${selected.modelId}` : "";
  const missing = selected && !rows.some((row) => modelKey(row) === value);
  const pending = catalog.isLoading || catalog.isValidating || refreshing;
  const status = pending
    ? "正在读取模型"
    : denied
      ? "暂无此类模型的生成权限"
      : catalog.error
        ? "模型读取失败，请重试"
        : "暂无可用模型";
  const showStatus = enabled && (pending || !rows.length);

  async function retry() {
    setRefreshing(true);
    try {
      if (denied) {
        const session = await refresh();
        if (!session) throw new Error("暂时无法读取权限，请稍后重试");
        // The new account scope owns its own request; never revalidate the old one.
        if (accountScope(session.account ?? null) !== scope) return;
      }
      if (identity.current === scope)
        await catalog.mutate(undefined, {
          revalidate: true,
          populateCache: false,
          // The SWR error state owns the notification, including failed retries.
          throwOnError: false,
        });
    } catch (error) {
      if (identity.current === scope) notifyError(error, "模型读取失败");
    } finally {
      setRefreshing(false);
    }
  }

  return (
    <div className="flex max-w-full min-w-0 items-center gap-1">
      <Select
        disabled={!enabled}
        value={value}
        onValueChange={(next) => {
          const model = rows.find((row) => modelKey(row) === next);
          if (model?.connectionId != null && model.modelId)
            onSelect({
              connectionId: model.connectionId,
              modelId: model.modelId,
            });
        }}
      >
        <SelectTrigger
          aria-label="生成模型"
          aria-describedby={showStatus ? statusId : undefined}
          title={showStatus ? status : undefined}
          className="max-w-64 min-w-0 flex-1"
        >
          <SelectValue placeholder={showStatus ? status : "选择模型"} />
        </SelectTrigger>
        <SelectContent onEscapeKeyDown={(event) => event.stopPropagation()}>
          <SelectGroup>
            {missing && (
              <SelectItem value={value} disabled>
                已保存的模型
              </SelectItem>
            )}
            {rows
              .filter((row) => row.connectionId != null && row.modelId)
              .map((row) => (
                <SelectItem key={modelKey(row)} value={modelKey(row)}>
                  {modelOptionLabel(row, rows)}
                </SelectItem>
              ))}
            {!rows.length && (
              <SelectItem value="unavailable" disabled>
                {status}
              </SelectItem>
            )}
          </SelectGroup>
        </SelectContent>
      </Select>
      {enabled && (catalog.error || catalog.data?.length === 0) && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="重新读取模型"
          disabled={refreshing || catalog.isValidating}
          onClick={() => void retry()}
        >
          <RefreshCw />
        </Button>
      )}
      {showStatus && (
        <p id={statusId} role="status" className="sr-only">
          {status}
        </p>
      )}
    </div>
  );
}

function modelKey(model: CanvasContract.ModelExecutionVO) {
  return `${model.connectionId}:${model.modelId}`;
}

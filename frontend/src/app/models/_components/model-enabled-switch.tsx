"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useSWRConfig } from "swr";
import {
  catalogMessage,
  useCatalogMutation,
} from "@/components/catalog/mutation";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { changeModelActivation } from "./model-activation";

export function useModelActivation(
  model: API.CreatedModel,
  onSaved: () => void,
) {
  const { mutate } = useSWRConfig();
  const mutation = useCatalogMutation(() => {}, `model.activation.${model.id}`);
  const controller = useRef<AbortController | null>(null);
  const [operation, setOperation] = useState<
    "enable" | "disable" | "update" | null
  >(null);
  const [error, setError] = useState("");
  useEffect(() => () => controller.current?.abort(), []);
  async function change(intent: "enable" | "disable" | "update") {
    if (!model.id || controller.current || mutation.busy || mutation.pending)
      return;
    const abort = new AbortController();
    controller.current = abort;
    setOperation(intent);
    setError("");
    try {
      await changeModelActivation(model.id, intent, mutation.run, abort.signal);
    } catch (error) {
      if (!abort.signal.aborted) setError(catalogMessage(error));
    } finally {
      controller.current = null;
      if (!abort.signal.aborted) {
        // Reconcile even after a conflict or unknown result; never guess the switch value.
        onSaved();
        void mutate(
          (key) =>
            Array.isArray(key) &&
            typeof key[0] === "string" &&
            ["canvas-node-models", "canvas-audio-model-catalog"].includes(
              key[0],
            ),
        ).catch(() => undefined);
        setOperation(null);
      }
    }
  }
  async function recover() {
    await mutation.recover();
    onSaved();
  }
  return {
    change,
    recover,
    operation,
    error,
    mutation,
    busy: !!operation || mutation.busy,
    blocked: !!operation || mutation.busy || !!mutation.pending,
  };
}

export function ModelEnabledSwitch({
  model,
  activation,
  disabled = false,
}: {
  model: API.CreatedModel;
  activation: ReturnType<typeof useModelActivation>;
  disabled?: boolean;
}) {
  const id = useId();
  const enabled = model.status === "active";
  const blocked = disabled || model.status === "archived" || activation.blocked;
  return (
    <div className="flex flex-col gap-2">
      <Field
        orientation="horizontal"
        data-disabled={blocked || undefined}
        aria-busy={activation.busy}
      >
        <FieldLabel htmlFor={id} className="min-h-11">
          {activation.busy ? <Spinner /> : null}
          <span className="text-muted-foreground">
            {activation.operation === "enable"
              ? "正在启用…"
              : activation.operation === "disable"
                ? "正在停用…"
                : enabled
                  ? "已启用"
                  : "未启用"}
          </span>
        </FieldLabel>
        <Switch
          id={id}
          aria-label={`启用 ${model.display_name ?? "模型"}`}
          checked={enabled}
          disabled={blocked}
          onCheckedChange={(checked) =>
            void activation.change(checked ? "enable" : "disable")
          }
        />
      </Field>
      {activation.error || activation.mutation.message ? (
        <FeedbackToast
          message={activation.error || activation.mutation.message}
          tone={activation.error ? "error" : "info"}
        />
      ) : null}
      {activation.mutation.pending && !activation.busy ? (
        <Button
          size="sm"
          variant="outline"
          onClick={() => void activation.recover()}
        >
          查询结果
        </Button>
      ) : null}
    </div>
  );
}

export function ModelRowEnabledSwitch({
  model,
  onSaved,
  disabled,
}: {
  model: API.CreatedModel;
  onSaved: () => void;
  disabled: boolean;
}) {
  const activation = useModelActivation(model, onSaved);
  return (
    <ModelEnabledSwitch
      model={model}
      activation={activation}
      disabled={disabled}
    />
  );
}

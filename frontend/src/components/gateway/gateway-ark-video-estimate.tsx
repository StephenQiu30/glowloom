"use client";

import { useState, type FormEvent } from "react";
import { Calculator } from "lucide-react";
import { estimateArkVideo } from "@/api/aiGateway";
import type {
  ArkVideoEstimateSpec,
  GatewayPreset,
} from "@/lib/ai-gateway-admin";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { FieldSet } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { InputField, SelectField } from "./gateway-fields";

type ArkPreset = GatewayPreset & { video_estimate: ArkVideoEstimateSpec };

export function GatewayArkVideoEstimate({
  presets,
}: {
  presets: GatewayPreset[] | undefined;
}) {
  const options = new Map(
    (presets ?? [])
      .filter((preset): preset is ArkPreset => Boolean(preset.video_estimate))
      .map((preset) => [preset.upstream, preset]),
  );
  const [model, setModel] = useState("");
  const preset = options.get(model) ?? options.values().next().value;
  if (!preset) return null;

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" className="max-md:min-h-11">
          <Calculator aria-hidden="true" />
          方舟费用预估
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>方舟费用预估</DialogTitle>
          <DialogDescription>
            按方舟公开价估算视频生成费用，实际费用以方舟任务用量为准。
          </DialogDescription>
        </DialogHeader>
        <SelectField
          label="方舟模型"
          description={preset.upstream}
          value={preset.upstream}
          options={[...options.values()].map(({ upstream, name }) => [
            upstream,
            name,
          ])}
          onChange={(event) => setModel(event.target.value)}
        />
        <ArkEstimateForm key={preset.upstream} preset={preset} />
      </DialogContent>
    </Dialog>
  );
}

function secondOptions(min: number, max: number) {
  return Array.from({ length: max - min + 1 }, (_, index) => {
    const seconds = min + index;
    return [String(seconds), `${seconds} 秒`] as const;
  });
}

function ArkEstimateForm({ preset }: { preset: ArkPreset }) {
  const spec = preset.video_estimate;
  const resolutions = Object.keys(spec.resolutions);
  const [resolution, setResolution] = useState(
    spec.resolutions["720p"] ? "720p" : resolutions[0],
  );
  const [result, setResult] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const ratios = Object.keys(spec.resolutions[resolution].geometries);

  async function calculate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    setResult("");
    try {
      const estimate = await estimateArkVideo({
        model: preset.upstream,
        resolution,
        ratio: String(fields.get("ratio")),
        reference_seconds: Number(fields.get("reference_seconds")),
        output_seconds: Number(fields.get("output_seconds")),
        count: Number(fields.get("count")),
      });
      if (!/^\d+\.\d{2}$/.test(estimate.amount_cny ?? "")) {
        throw new Error("费用响应无效");
      }
      setResult(estimate.amount_cny ?? "");
    } catch {
      setError("暂时无法计算费用，请检查规格后重试。");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className="grid gap-4"
      onSubmit={calculate}
      onChange={() => {
        setError("");
        setResult("");
      }}
    >
      <FieldSet disabled={busy} className="grid grid-cols-2 gap-3">
        <SelectField
          label="参考视频总时长"
          name="reference_seconds"
          options={[
            ["0", "无参考视频"],
            ...secondOptions(
              spec.min_reference_seconds,
              spec.max_reference_seconds,
            ),
          ]}
        />
        <SelectField
          label="输出时长"
          name="output_seconds"
          defaultValue={String(spec.min_output_seconds)}
          options={secondOptions(
            spec.min_output_seconds,
            spec.max_output_seconds,
          )}
        />
        <SelectField
          label="分辨率"
          value={resolution}
          options={resolutions.map((option) => [option, option.toUpperCase()])}
          onChange={(event) => setResolution(event.target.value)}
        />
        <SelectField
          label="画幅"
          name="ratio"
          defaultValue={ratios.includes("16:9") ? "16:9" : ratios[0]}
          options={ratios.map((ratio) => [ratio, ratio])}
        />
        <InputField
          label="生成条数"
          name="count"
          type="number"
          min={1}
          step={1}
          value={1}
        />
      </FieldSet>
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex items-center justify-between gap-3">
        <div role="status" className="text-sm">
          {result && (
            <>
              预计费用{" "}
              <span className="font-semibold tabular-nums">约 ¥{result}</span>
            </>
          )}
        </div>
        <Button type="submit" disabled={busy}>
          {busy ? "计算中" : "计算费用"}
        </Button>
      </div>
      {preset.price_source_url && (
        <a href={preset.price_source_url} className="w-fit text-sm underline">
          方舟官方价目
        </a>
      )}
    </form>
  );
}

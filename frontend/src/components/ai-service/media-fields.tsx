"use client";

import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { listProjects } from "@/api/projects";
import { AiServiceAction } from "./admin-action";
import {
  FieldDescription,
  FieldGroup,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { Input } from "@/components/ui/input";
import type {
  GatewayModel,
  GatewayParameter,
  GatewayVideoMetering,
  ParameterType,
  ParameterValue,
} from "@/lib/ai-service-admin";
import {
  FormDetails,
  InputField,
  SelectField,
  SetupSelectField,
  TextField,
  num,
  text,
  uuidPattern,
} from "./admin-fields";

export const capabilities = [
  ["image.generate", "图像生成"],
  ["video.generate", "视频生成"],
  ["audio.generate", "音频生成"],
] as const;

const knownModes = [
  ["text2image", "文生图"],
  ["image2image", "图生图"],
  ["text2video", "文生视频"],
  ["image2video", "图生视频"],
  ["text2audio", "文生音频"],
] as const;

export function modeLabel(mode: string) {
  return knownModes.find(([key]) => key === mode)?.[1] ?? mode;
}

const parameterTypes: readonly (readonly [ParameterType, string])[] = [
  ["string", "文本"],
  ["integer", "整数"],
  ["number", "小数"],
  ["boolean", "是 / 否"],
];

export type MediaDraft = {
  capability: string;
  modes: string[];
  parameters: Record<string, GatewayParameter>;
  output_policy?: string;
};

export function MediaModelFields({
  capability,
  onCapabilityChange,
}: {
  capability: string;
  onCapabilityChange: (capability: string) => void;
}) {
  const [modes, setModes] = useState([0]);
  const [params, setParams] = useState<number[]>([]);
  const [sequence, setSequence] = useState(1);

  function addRow(setRows: React.Dispatch<React.SetStateAction<number[]>>) {
    setRows((rows) => [...rows, sequence]);
    setSequence((value) => value + 1);
  }
  return (
    <FieldGroup>
      <SelectField
        label="生成类型"
        name="capability"
        options={capabilities}
        value={capability}
        onChange={(event) => onCapabilityChange(event.currentTarget.value)}
      />
      <FieldSet>
        <FieldLegend variant="label">支持的生成模式</FieldLegend>
        <FieldDescription>
          选择常见模式，或填写适配器支持的模式标识。每种模式都需要验证。
        </FieldDescription>
        <FieldGroup>
          {modes.map((id, index) => (
            <ModeRow
              key={id}
              index={index}
              capability={capability}
              onRemove={
                modes.length > 1
                  ? () => setModes((rows) => rows.filter((key) => key !== id))
                  : undefined
              }
            />
          ))}
          <AddButton
            label="添加模式"
            disabled={modes.length >= 16}
            onClick={() => addRow(setModes)}
          />
        </FieldGroup>
      </FieldSet>
      <FormDetails label="生成参数与默认值">
        <FieldGroup>
          {params.map((id, index) => (
            <ParameterRow
              key={id}
              index={index}
              onRemove={() =>
                setParams((rows) => rows.filter((key) => key !== id))
              }
            />
          ))}
          <AddButton label="添加参数" onClick={() => addRow(setParams)} />
        </FieldGroup>
      </FormDetails>
    </FieldGroup>
  );
}

export function VideoMeterFields({
  resolutions,
  resolution,
  onResolutionChange,
}: {
  resolutions: string[];
  resolution: string;
  onResolutionChange: (resolution: string) => void;
}) {
  const [rows, setRows] = useState(["initial"]);
  return (
    <FieldSet>
      <FieldLegend variant="label">视频 Token 计量</FieldLegend>
      <FieldDescription>
        填写已核验的估量参数。输出分辨率和画幅从这里生成，时长由创作者选择。
      </FieldDescription>
      <FieldGroup className="grid sm:grid-cols-2">
        {resolutions.length > 0 ? (
          <SetupSelectField
            label="输出分辨率"
            name="video_resolution"
            value={resolution}
            options={resolutions.map((option) => [option, option])}
            placeholder="选择分辨率"
            onValueChange={onResolutionChange}
            required
          />
        ) : (
          <InputField
            label="输出分辨率"
            name="video_resolution"
            onChange={(event) => onResolutionChange(event.currentTarget.value)}
          />
        )}
        <InputField label="计量帧率" name="meter_fps" type="number" min={1} />
        <InputField
          label="每 Token 像素量"
          name="pixels_per_token"
          type="number"
          min={1}
        />
        <InputField
          label="最低计费 Token"
          name="minimum_tokens"
          type="number"
          min={0}
          required={false}
        />
        <InputField
          label="最短生成时长（秒）"
          name="video_min_seconds"
          type="number"
          min={1}
        />
        <InputField
          label="最长生成时长（秒）"
          name="video_max_seconds"
          type="number"
          min={1}
        />
        <InputField
          label="单次视频 Token 上限"
          name="max_output"
          type="number"
          min={1}
        />
      </FieldGroup>
      <FieldGroup>
        {rows.map((id, index) => (
          <FieldGroup
            key={id}
            className="grid items-end sm:grid-cols-[1fr_1fr_1fr_auto]"
          >
            <Input type="hidden" name="geometry_row" value={id} readOnly />
            <InputField label={`画幅 ${index + 1}`} name={`ratio_${id}`} />
            <InputField
              label={`宽度 ${index + 1}`}
              name={`width_${id}`}
              type="number"
              min={1}
            />
            <InputField
              label={`高度 ${index + 1}`}
              name={`height_${id}`}
              type="number"
              min={1}
            />
            {rows.length > 1 && (
              <RemoveButton
                label={`移除画幅 ${index + 1}`}
                onClick={() =>
                  setRows((items) => items.filter((item) => item !== id))
                }
              />
            )}
          </FieldGroup>
        ))}
        <AddButton
          label="添加画幅"
          onClick={() => setRows((items) => [...items, crypto.randomUUID()])}
        />
      </FieldGroup>
    </FieldSet>
  );
}

export function readVideoMetering(form: FormData, draft: MediaDraft) {
  const rows = form.getAll("geometry_row");
  const ratios = rows.map((id) => text(form, `ratio_${id}`));
  const resolution = text(form, "video_resolution");
  const min = num(form, "video_min_seconds");
  const max = num(form, "video_max_seconds");
  if (
    !ratios.length ||
    new Set(ratios).size !== ratios.length ||
    min < 1 ||
    min > max
  ) {
    throw new Error("请填写不同画幅和有效的生成时长范围。");
  }
  const metering: GatewayVideoMetering = {
    resolution,
    frames_per_second: num(form, "meter_fps"),
    pixels_per_token: num(form, "pixels_per_token"),
    minimum_tokens: num(form, "minimum_tokens"),
    geometries: Object.fromEntries(
      rows.map((id, i) => [
        ratios[i],
        { width: num(form, `width_${id}`), height: num(form, `height_${id}`) },
      ]),
    ),
  };
  const parameters: MediaDraft["parameters"] = {
    resolution: { type: "string", default: resolution, enum: [resolution] },
    ratio: { type: "string", default: ratios[0], enum: ratios },
    duration: { type: "integer", min, max },
  };
  if (Object.keys(parameters).some((key) => key in draft.parameters)) {
    throw new Error("分辨率、画幅和时长请在视频 Token 计量中设置。");
  }
  return {
    ...draft,
    parameters: { ...draft.parameters, ...parameters },
    video_metering: metering,
  };
}

function AddButton({
  label,
  disabled,
  onClick,
}: {
  label: string;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <AiServiceAction
      label={label}
      icon={Plus}
      disabled={disabled}
      onClick={onClick}
    />
  );
}

function RemoveButton({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return <AiServiceAction label={label} icon={Trash2} onClick={onClick} />;
}

function ModeRow({
  index,
  capability,
  onRemove,
}: {
  index: number;
  capability: string;
  onRemove?: () => void;
}) {
  // Change of capability remounts the selector with a compatible initial option.
  return (
    <ModeSelection
      key={capability}
      index={index}
      capability={capability}
      onRemove={onRemove}
    />
  );
}

function ModeSelection({
  index,
  capability,
  onRemove,
}: {
  index: number;
  capability: string;
  onRemove?: () => void;
}) {
  const options = knownModes.filter(([key]) =>
    key.endsWith(capability.split(".")[0]),
  );
  const [value, setValue] = useState<string>(
    index === 0 ? (options[0]?.[0] ?? "custom") : "",
  );
  return (
    <div className="flex items-end gap-2">
      <FieldGroup className="min-w-0 flex-1">
        <SelectField
          label={`生成模式 ${index + 1}`}
          name="mode"
          value={value}
          onChange={(event) => setValue(event.currentTarget.value)}
          required
          options={[["", "选择模式"], ...options, ["custom", "自定义模式"]]}
        />
        {value === "custom" && (
          <InputField
            label="自定义模式标识"
            name={`mode_custom_${index}`}
            maxLength={160}
          />
        )}
      </FieldGroup>
      {onRemove && (
        <RemoveButton label={`移除模式 ${index + 1}`} onClick={onRemove} />
      )}
    </div>
  );
}

function ParameterRow({
  index,
  onRemove,
}: {
  index: number;
  onRemove: () => void;
}) {
  const [kind, setKind] = useState<ParameterType>("string");
  const [choices, setChoices] = useState<string[]>([]);
  const prefix = `param_${index}`;
  const numeric = kind === "integer" || kind === "number";
  return (
    <FieldSet className="min-w-0">
      <FieldLegend variant="label">参数 {index + 1}</FieldLegend>
      <FieldGroup>
        <Input type="hidden" name="parameter_index" value={index} />
        <FieldGroup className="grid sm:grid-cols-[1fr_1fr_auto]">
          <InputField
            label="参数名称"
            name={`${prefix}_name`}
            placeholder="例如 size"
            maxLength={160}
          />
          <SelectField
            label="参数类型"
            name={`${prefix}_type`}
            options={parameterTypes}
            value={kind}
            onChange={(event) =>
              setKind(event.currentTarget.value as ParameterType)
            }
          />
          <RemoveButton label={`移除参数 ${index + 1}`} onClick={onRemove} />
        </FieldGroup>
        {numeric && (
          <FieldGroup className="grid sm:grid-cols-2">
            <InputField
              label="最小值"
              name={`${prefix}_min`}
              type="number"
              step={kind === "integer" ? 1 : "any"}
              required={false}
            />
            <InputField
              label="最大值"
              name={`${prefix}_max`}
              type="number"
              step={kind === "integer" ? 1 : "any"}
              required={false}
            />
          </FieldGroup>
        )}
        <ScalarField
          key={kind}
          label="默认值"
          name={`${prefix}_default`}
          parameter={{ type: kind }}
          hint="不填时由请求指定或使用供应商默认值。"
        />
        {choices.map((id, choice) => (
          <div className="flex items-end gap-2" key={id}>
            <FieldGroup className="min-w-0 flex-1">
              <ScalarField
                key={kind}
                label={`可选值 ${choice + 1}`}
                name={`${prefix}_enum`}
                parameter={{ type: kind }}
                required
              />
            </FieldGroup>
            <RemoveButton
              label={`移除可选值 ${choice + 1}`}
              onClick={() =>
                setChoices((items) => items.filter((key) => key !== id))
              }
            />
          </div>
        ))}
        <AddButton
          label="添加可选值"
          onClick={() => setChoices((items) => [...items, crypto.randomUUID()])}
        />
        <Separator />
      </FieldGroup>
    </FieldSet>
  );
}

function ScalarField({
  label,
  name,
  parameter,
  hint,
  required,
}: {
  label: string;
  name: string;
  parameter: GatewayParameter;
  hint?: string;
  required?: boolean;
}) {
  const { type, min, max, enum: options } = parameter;
  if (options?.length || type === "boolean") {
    const scalarOptions = options ?? [true, false];
    return (
      <SelectField
        label={label}
        name={name}
        required={required}
        description={hint}
        options={[
          ["", required ? "请选择" : "使用默认值"],
          ...scalarOptions.map(
            (v) =>
              [
                String(v),
                typeof v === "boolean" ? (v ? "是" : "否") : String(v),
              ] as const,
          ),
        ]}
      />
    );
  }
  return (
    <InputField
      label={label}
      name={name}
      description={hint}
      required={required ?? false}
      type={type === "string" ? "text" : "number"}
      step={type === "integer" ? 1 : "any"}
      maxLength={type === "string" ? 10000 : undefined}
      min={min}
      max={max}
    />
  );
}

function parseScalar(raw: string, p: GatewayParameter): ParameterValue {
  switch (p.type) {
    case "string":
      return raw;
    case "boolean":
      if (raw === "true" || raw === "false") return raw === "true";
      break;
    case "integer":
    case "number": {
      const n = Number(raw);
      if (
        Number.isFinite(n) &&
        Math.abs(n) <= 2 ** 40 &&
        (p.type !== "integer" || Number.isInteger(n)) &&
        (p.min === undefined || n >= p.min) &&
        (p.max === undefined || n <= p.max)
      )
        return n;
      break;
    }
  }
  throw new Error("请检查生成参数的类型和允许范围。");
}

export function readMediaModel(form: FormData): MediaDraft {
  const modes = form
    .getAll("mode")
    .map((mode, index) =>
      mode === "custom" ? text(form, `mode_custom_${index}`) : String(mode),
    );
  if (modes.some((mode) => !mode) || new Set(modes).size !== modes.length)
    throw new Error("请选择不同的生成模式。");
  const entries = form.getAll("parameter_index").map((index) => {
    const prefix = `param_${index}`;
    const name = text(form, `${prefix}_name`);
    const type = text(form, `${prefix}_type`) as ParameterType;
    if (!name || !parameterTypes.some(([key]) => key === type))
      throw new Error("请填写参数名称和类型。");
    const p: GatewayParameter = { type };
    const min = text(form, `${prefix}_min`);
    const max = text(form, `${prefix}_max`);
    if (min) p.min = Number(parseScalar(min, p));
    if (max) p.max = Number(parseScalar(max, { type }));
    if (p.min !== undefined && p.max !== undefined && p.min > p.max)
      throw new Error(`参数 ${name} 的最小值须小于或等于最大值。`);
    const values = form
      .getAll(`${prefix}_enum`)
      .map((value) => parseScalar(String(value), p));
    if (values.length) p.enum = [...new Set(values)];
    const raw = form.get(`${prefix}_default`);
    if (typeof raw === "string" && raw !== "") {
      p.default = parseScalar(raw, p);
      if (p.enum && !p.enum.includes(p.default))
        throw new Error(`参数 ${name} 的默认值必须在可选值中。`);
    }
    return [name, p] as const;
  });
  if (new Set(entries.map(([name]) => name)).size !== entries.length)
    throw new Error("生成参数名称不能重复。");
  return {
    capability: text(form, "capability"),
    modes,
    parameters: Object.fromEntries(entries),
    ...(form.get("reviewed") === "on"
      ? { output_policy: "provider_success_v1" }
      : {}),
  };
}

const referenceRoles = [
  "image",
  "video",
  "audio",
  "first_frame",
  "last_frame",
  "reference_image",
  "reference_video",
  "reference_audio",
  "character_image",
  "driving_video",
  "driving_audio",
] as const;

export function MediaProbeFields({ model }: { model: GatewayModel }) {
  return (
    <FieldGroup>
      <TestProjectField />
      {model.modes?.map((mode, index) => (
        <ProbeFields
          key={mode}
          mode={mode}
          index={index}
          parameters={model.parameters ?? {}}
        />
      ))}
    </FieldGroup>
  );
}

function TestProjectField() {
  const [projects, setProjects] = useState<[string, string][]>([]);
  const [selected, setSelected] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void listProjects(
      { status: "active", limit: 100 },
      { signal: controller.signal },
    ).then(
      (response) => {
        if (controller.signal.aborted) return;
        setProjects(
          (response?.items ?? []).flatMap(({ id, name }) =>
            id && name ? [[id, name]] : [],
          ),
        );
      },
      () => {
        /* The explicit project identifier remains available when the list cannot load. */
      },
    );
    return () => controller.abort();
  }, []);
  return (
    <FieldGroup>
      {projects.length > 0 && (
        <SelectField
          label="验证项目"
          name={selected ? "project_id" : undefined}
          options={[["", "手动填写项目 ID"], ...projects]}
          value={selected}
          onChange={(event) => setSelected(event.currentTarget.value)}
          description="引用素材须来自同一个测试项目。"
        />
      )}
      {!selected && (
        <InputField
          label="验证项目 ID"
          name="project_id"
          pattern={uuidPattern}
          description="使用测试付款空间下的创作项目。引用素材须来自该项目。"
        />
      )}
    </FieldGroup>
  );
}

function ProbeFields({
  mode,
  index,
  parameters,
}: {
  mode: string;
  index: number;
  parameters: Record<string, GatewayParameter>;
}) {
  const [refs, setRefs] = useState<string[]>([]);
  const prefix = `probe_${index}`;
  return (
    <FieldSet className="min-w-0">
      <FieldLegend>{modeLabel(mode)} · 验证用例</FieldLegend>
      <FieldGroup>
        {mode !== "motion" && (
          <TextField
            label="验证提示词"
            name={`${prefix}_prompt`}
            defaultValue="Glowloom 模型接入验证"
            required={mode !== "avatar"}
            maxLength={mode === "avatar" ? 300 : 16000}
          />
        )}
        {!!Object.keys(parameters).length && (
          <FieldGroup className="grid sm:grid-cols-2">
            {Object.entries(parameters).map(([key, parameter]) => (
              <ScalarField
                key={key}
                label={key}
                name={`${prefix}_param_${key}`}
                parameter={parameter}
                hint={
                  parameter.default !== undefined
                    ? `默认 ${String(parameter.default)}`
                    : undefined
                }
              />
            ))}
          </FieldGroup>
        )}
        {refs.map((id, ref) => (
          <FieldGroup key={id}>
            <Input type="hidden" name={`${prefix}_references`} value={ref} />
            <FieldGroup className="grid sm:grid-cols-2">
              <InputField
                name={`${prefix}_ref_${ref}_id`}
                label="素材 ID"
                pattern={uuidPattern}
              />
              <InputField
                name={`${prefix}_ref_${ref}_version`}
                label="素材版本"
                maxLength={160}
              />
              <ReferenceRole name={`${prefix}_ref_${ref}_role`} />
              <InputField
                name={`${prefix}_ref_${ref}_binding`}
                label="绑定 ID（可选）"
                pattern={uuidPattern}
                required={false}
              />
            </FieldGroup>
            <RemoveButton
              label={`移除测试素材 ${ref + 1}`}
              onClick={() =>
                setRefs((items) => items.filter((key) => key !== id))
              }
            />
          </FieldGroup>
        ))}
        <AddButton
          label="添加测试素材"
          disabled={refs.length >= 16}
          onClick={() => setRefs((items) => [...items, crypto.randomUUID()])}
        />
        <Separator />
      </FieldGroup>
    </FieldSet>
  );
}

function ReferenceRole({ name }: { name: string }) {
  const [role, setRole] = useState("");
  const labels = [
    "图像",
    "视频",
    "音频",
    "首帧",
    "尾帧",
    "参考图像",
    "参考视频",
    "参考音频",
    "角色图像",
    "驱动视频",
    "驱动音频",
  ];
  return (
    <FieldGroup>
      <SelectField
        label="素材用途"
        name={role === "custom" ? undefined : name}
        value={role}
        required
        onChange={(event) => setRole(event.currentTarget.value)}
        options={[
          ["", "选择素材用途"],
          ...referenceRoles.map((key, index) => [key, labels[index]] as const),
          ["custom", "自定义用途"],
        ]}
      />
      {role === "custom" && (
        <InputField label="用途标识" name={name} maxLength={64} />
      )}
    </FieldGroup>
  );
}

export function readMediaProbes(form: FormData, model: GatewayModel) {
  return (model.modes ?? []).map((mode, index) => {
    const prefix = `probe_${index}`;
    const params = Object.fromEntries(
      Object.entries(model.parameters ?? {}).flatMap(([key, parameter]) => {
        const raw = form.get(`${prefix}_param_${key}`);
        return typeof raw === "string" && raw !== ""
          ? [[key, parseScalar(raw, parameter)]]
          : [];
      }),
    );
    const references = form.getAll(`${prefix}_references`).map((ref) => {
      const base = `${prefix}_ref_${ref}`;
      const binding = text(form, `${base}_binding`);
      return {
        id: text(form, `${base}_id`),
        version: text(form, `${base}_version`),
        role: text(form, `${base}_role`),
        ...(binding ? { binding_id: binding } : {}),
      };
    });
    return {
      model: model.name,
      capability: model.capability,
      mode,
      prompt: text(form, `${prefix}_prompt`),
      params,
      output_count: 1,
      ...(references.length ? { references } : {}),
    };
  });
}

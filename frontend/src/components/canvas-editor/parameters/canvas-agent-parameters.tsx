"use client";

import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export const defaultCanvasAgent: CanvasContract.CanvasAgentConfigurationDTO = {
  profile: "fixed-research-mock@1",
  goal: "整理连接的资料，并标注来源",
  maxRounds: 4,
  maxTools: 24,
  parallelism: 2,
};

export function CanvasAgentParameters({
  id,
  value,
  profiles,
  onChange,
}: {
  id: string;
  value: CanvasContract.CanvasAgentConfigurationDTO;
  profiles: CanvasContract.WorkflowAgentProfileVO[];
  onChange: (value: CanvasContract.CanvasAgentConfigurationDTO) => void;
}) {
  return (
    <FieldGroup className="p-3">
      <Field>
        <FieldLabel htmlFor={`${id}-agent-profile`}>执行场景</FieldLabel>
        <Select
          value={value.profile}
          onValueChange={(profile) =>
            onChange({
              ...value,
              profile,
              maxRounds: profile === "script-analysis-mock@1" ? 1 : 4,
              maxTools: profile === "script-analysis-mock@1" ? 0 : 24,
            })
          }
        >
          <SelectTrigger id={`${id}-agent-profile`}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {profiles.map((profile) => (
                <SelectItem key={profile.id} value={profile.id!}>
                  {profile.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <FieldDescription>
          {
            profiles.find((profile) => profile.id === value.profile)
              ?.description
          }
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-agent-goal`}>Agent 目标</FieldLabel>
        <Textarea
          id={`${id}-agent-goal`}
          value={value.goal}
          maxLength={4000}
          onChange={(event) => onChange({ ...value, goal: event.target.value })}
        />
        <FieldDescription>
          读取节点正文及连接的输入，使用所选场景产出结果。本模式不会调用付费模型。
        </FieldDescription>
      </Field>
      {(
        [
          ["maxRounds", "模型轮次上限", [1, 2, 3, 4, 5, 6, 7, 8]],
          ["maxTools", "工具调用上限", [0, 2, 4, 8, 16, 24, 32]],
          ["parallelism", "并行工具数", [1, 2, 3, 4]],
        ] as const
      ).map(([key, label, choices]) => (
        <Field key={key}>
          <FieldLabel htmlFor={`${id}-agent-${key}`}>{label}</FieldLabel>
          <Select
            value={String(value[key])}
            onValueChange={(next) =>
              onChange({ ...value, [key]: Number(next) })
            }
          >
            <SelectTrigger id={`${id}-agent-${key}`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {choices
                  .filter(
                    (choice) =>
                      value.profile !== "fixed-research-mock@1" ||
                      key === "parallelism" ||
                      choice >= (key === "maxRounds" ? 3 : 2),
                  )
                  .map((choice) => (
                    <SelectItem key={choice} value={String(choice)}>
                      {choice}
                    </SelectItem>
                  ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      ))}
    </FieldGroup>
  );
}

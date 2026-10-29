"use client";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useId } from "react";
export const voiceCategories = {
  gender: { label: "性别", values: ["未标注", "男声", "女声"] },
  age: { label: "年龄", values: ["未标注", "儿童", "青年", "中年", "老年"] },
  language: {
    label: "语言",
    values: ["未标注", "中文", "英文", "日文", "其他"],
  },
  style: {
    label: "声音特点",
    values: ["未标注", "自然", "温柔", "活泼", "沉稳", "旁白"],
  },
};
export type VoiceTags = Record<keyof typeof voiceCategories, string>;
export const emptyVoiceTags: VoiceTags = {
  gender: "未标注",
  age: "未标注",
  language: "未标注",
  style: "未标注",
};
export function VoiceFilters({
  value,
  onChange,
  filter = false,
  disabled = false,
}: {
  value: VoiceTags;
  onChange: (v: VoiceTags) => void;
  filter?: boolean;
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      {(Object.keys(voiceCategories) as (keyof VoiceTags)[]).map((key) => (
        <Field key={key} className="gap-1">
          <FieldLabel htmlFor={`${id}-${key}`}>
            {voiceCategories[key].label}
          </FieldLabel>
          <Select
            disabled={disabled}
            value={value[key]}
            onValueChange={(v) => onChange({ ...value, [key]: v })}
          >
            <SelectTrigger id={`${id}-${key}`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {filter && (
                <SelectItem value="all" className="max-sm:min-h-11">
                  全部
                </SelectItem>
              )}
              {voiceCategories[key].values.map((v) => (
                <SelectItem key={v} value={v} className="max-sm:min-h-11">
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      ))}
    </div>
  );
}

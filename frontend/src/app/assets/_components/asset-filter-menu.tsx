"use client";

import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { AssetFilters, AssetKind } from "./asset-query";

type Choice = { value: string; label: string };
const choices = (values: string[]): Choice[] =>
  values.map((value) => ({ value, label: value }));
const imageRatios = choices([
  "21:9",
  "16:9",
  "3:2",
  "4:3",
  "1:1",
  "3:4",
  "2:3",
  "9:16",
]);
const videoRatios = choices(["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"]);
const videoModes = [
  { value: "all_reference", label: "全能参考" },
  { value: "first_last_frame", label: "首尾帧" },
  { value: "multi_frame", label: "智能多帧" },
  { value: "lip_sync", label: "对口型" },
  { value: "motion_mimic", label: "动作模仿" },
];
const audioModes = [
  { value: "song", label: "人声歌曲" },
  { value: "instrumental", label: "纯音乐" },
  { value: "tts", label: "AI 配音" },
];

export function AssetFilterMenu({
  kind,
  value,
  disabled,
  onChange,
}: {
  kind?: AssetKind;
  value: AssetFilters;
  disabled: boolean;
  onChange: (value: AssetFilters) => void;
}) {
  const groups: {
    label: string;
    field: "processTypes" | "resolutions" | "ratios" | "generationModes";
    choices: Choice[];
  }[] = [];
  if (kind === "image" || kind === "video") {
    groups.push({
      label: "类型",
      field: "processTypes",
      choices: [
        { value: "super_resolution", label: "超清" },
        ...(kind === "video"
          ? [{ value: "frame_interpolation", label: "补帧" }]
          : []),
      ],
    });
    groups.push({
      label: "分辨率",
      field: "resolutions",
      choices: choices(
        kind === "image"
          ? ["1K", "2K", "4K", "8K"]
          : ["720P", "1080P", "2K", "4K"],
      ),
    });
    groups.push({
      label: "比例",
      field: "ratios",
      choices: kind === "image" ? imageRatios : videoRatios,
    });
  }
  if (kind === "video" || kind === "audio")
    groups.push({
      label: "生成模式",
      field: "generationModes",
      choices: kind === "video" ? videoModes : audioModes,
    });
  const active =
    value.favorite || groups.some((group) => value[group.field].length > 0);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant={active ? "secondary" : "ghost"}
          disabled={disabled}
          className="min-h-11"
          aria-label={value.favorite ? "筛选资产：收藏" : "筛选资产"}
        >
          {value.favorite ? "收藏" : "筛选"}
          <ChevronDown data-icon="inline-end" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64">
        <DropdownMenuGroup>
          <DropdownMenuLabel>操作</DropdownMenuLabel>
          <DropdownMenuCheckboxItem
            checked={value.favorite}
            onSelect={(event) => event.preventDefault()}
            onCheckedChange={(favorite) =>
              onChange({ ...value, favorite: favorite === true })
            }
          >
            收藏
          </DropdownMenuCheckboxItem>
        </DropdownMenuGroup>
        {groups.map((group) => (
          <DropdownMenuGroup key={group.field}>
            <DropdownMenuLabel>{group.label}</DropdownMenuLabel>
            {group.choices.map((choice) => (
              <DropdownMenuCheckboxItem
                key={choice.value}
                checked={value[group.field].includes(choice.value)}
                onSelect={(event) => event.preventDefault()}
                onCheckedChange={(checked) =>
                  onChange({
                    ...value,
                    [group.field]:
                      checked === true
                        ? [...value[group.field], choice.value]
                        : value[group.field].filter(
                            (item) => item !== choice.value,
                          ),
                  })
                }
              >
                {choice.label}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

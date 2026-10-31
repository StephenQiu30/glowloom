"use client";
import { CalendarDays, ListFilter } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FieldSet, FieldLegend } from "@/components/ui/field";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  defaultProjectPickerFilter,
  type ProjectPickerFilter,
} from "./project-picker";
export function ProjectPickerFilters({
  value,
  onChange,
  kind,
}: {
  value: ProjectPickerFilter;
  onChange: (value: ProjectPickerFilter) => void;
  kind: string;
}) {
  const count =
    Number(!!value.origin) +
    (kind === "image" ? value.resolutions.length + value.ratios.length : 0);
  return (
    <>
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant={value.from || value.through ? "secondary" : "ghost"}
            size="icon"
            aria-label="时间筛选"
          >
            <CalendarDays />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          data-canvas-no-zoom
          onMouseDown={(e) => e.stopPropagation()}
          className="space-y-3"
        >
          <p className="text-sm font-medium">创建时间</p>
          <Label htmlFor="picker-date-from">开始日期</Label>
          <Input
            id="picker-date-from"
            type="date"
            value={value.from}
            onChange={(e) => onChange({ ...value, from: e.target.value })}
          />
          <Label htmlFor="picker-date-through">结束日期（包含当天）</Label>
          <Input
            id="picker-date-through"
            type="date"
            value={value.through}
            onChange={(e) => onChange({ ...value, through: e.target.value })}
          />
        </PopoverContent>
      </Popover>
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant={count > 0 ? "secondary" : "ghost"}
            size={count ? "sm" : "icon"}
            aria-label={
              count ? `来源与规格筛选，已选 ${count} 项` : "来源与规格筛选"
            }
          >
            <ListFilter />
            {count > 0 && <span aria-hidden="true">{count}</span>}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          data-canvas-no-zoom
          onMouseDown={(e) => e.stopPropagation()}
          className="space-y-3"
        >
          <FilterSelect
            label="素材来源"
            value={value.origin}
            options={[
              ["upload", "上传"],
              ["generated", "生成"],
              ["system", "系统"],
            ]}
            onChange={(origin) =>
              onChange({
                ...value,
                origin: origin as ProjectPickerFilter["origin"],
              })
            }
          />
          {kind === "image" && (
            <>
              <FilterMultiSelect
                label="图片比例"
                value={value.ratios}
                options={[
                  "21:9",
                  "16:9",
                  "3:2",
                  "4:3",
                  "1:1",
                  "3:4",
                  "2:3",
                  "9:16",
                ]}
                onChange={(ratios) => onChange({ ...value, ratios })}
              />
              <FilterMultiSelect
                label="实际分辨率"
                value={value.resolutions}
                options={["1K", "2K", "4K", "8K"]}
                onChange={(resolutions) => onChange({ ...value, resolutions })}
              />
              <p className="text-xs text-muted-foreground">
                按图片实际宽高及长边 1024、2048、4096、8192 像素筛选。
              </p>
            </>
          )}
          <p className="text-xs text-muted-foreground">
            项目素材暂不支持收藏筛选；超分记录尚未接入。
          </p>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onChange(defaultProjectPickerFilter)}
          >
            清除筛选
          </Button>
        </PopoverContent>
      </Popover>
    </>
  );
}
function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: string[][];
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-1">
      <span className="text-sm">{label}</span>
      <Select
        value={value || "all"}
        onValueChange={(v) => onChange(v === "all" ? "" : v)}
      >
        <SelectTrigger aria-label={label} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent
          data-canvas-no-zoom
          onMouseDown={(e) => e.stopPropagation()}
        >
          <SelectItem value="all">全部</SelectItem>
          {options.map(([key, title]) => (
            <SelectItem key={key} value={key}>
              {title}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function FilterMultiSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string[];
  options: string[];
  onChange: (values: string[]) => void;
}) {
  return (
    <FieldSet className="gap-2 space-y-0" aria-label={label}>
      <FieldLegend variant="label" className="mb-0">
        {label}
      </FieldLegend>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground" aria-live="polite">
          已选 {value.length}
        </span>
        <Button
          size="sm"
          variant="ghost"
          disabled={!value.length}
          aria-label={`清空${label}`}
          onClick={() => onChange([])}
        >
          清空
        </Button>
      </div>
      <div className="grid grid-cols-2 gap-2">
        {options.map((option) => (
          <Label
            key={option}
            className="flex cursor-pointer items-center gap-2"
          >
            <Checkbox
              checked={value.includes(option)}
              onCheckedChange={(checked) =>
                onChange(
                  checked === true
                    ? options.filter(
                        (item) => item === option || value.includes(item),
                      )
                    : value.filter((item) => item !== option),
                )
              }
            />
            {option}
          </Label>
        ))}
      </div>
    </FieldSet>
  );
}

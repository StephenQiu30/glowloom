"use client";

import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import {
  CalendarDays,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { subjectDateRange } from "./unified-contracts";

function localDay(year: number, month: number, day: number) {
  const value = new Date(2000, 0, 1, 12);
  value.setFullYear(year, month, day);
  return value;
}
function dateText(value: Date) {
  return `${String(value.getFullYear()).padStart(4, "0")}-${String(value.getMonth() + 1).padStart(2, "0")}-${String(value.getDate()).padStart(2, "0")}`;
}
function parseDay(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  return localDay(year, month - 1, day);
}
function moveMonth(value: Date, offset: number) {
  const first = localDay(value.getFullYear(), value.getMonth() + offset, 1);
  const last = localDay(first.getFullYear(), first.getMonth() + 1, 0).getDate();
  return localDay(
    first.getFullYear(),
    first.getMonth(),
    Math.min(value.getDate(), last),
  );
}
const validYear = (value: Date) =>
  value.getFullYear() >= 1 && value.getFullYear() <= 9999;
const shortDate = (value: string) => value.slice(2);
const presets = ["全部", "最近一周", "最近一个月", "最近三个月"] as const;
type Endpoint = "start" | "end";

export function SubjectDateRange({
  from,
  through,
  disabled,
  onChange,
}: {
  from: string;
  through: string;
  disabled: boolean;
  onChange: (from: string, through: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState<Endpoint | null>(null);
  const [focused, setFocused] = useState(() => dateText(new Date()));
  const [month, setMonth] = useState(() => new Date());
  const [preset, setPreset] = useState<{
    label: string;
    from: string;
    through: string;
  } | null>(null);
  const [customRange, setCustomRange] = useState({ from, through });
  const calendar = useRef<HTMLTableElement>(null);
  const focusRequested = useRef(false);
  const today = dateText(new Date());
  const start = customRange.from;
  const end = customRange.through || today;
  const active = Boolean(from || through);
  const selectedPreset = !active
    ? "全部"
    : preset?.from === from && preset.through === through
      ? preset.label
      : "";
  const triggerLabel = !active
    ? "时间"
    : selectedPreset ||
      `${from ? shortDate(from) : "开始日期"} 至 ${shortDate(through || today)}`;
  useEffect(() => {
    if (target && focusRequested.current) {
      calendar.current
        ?.querySelector<HTMLButtonElement>(`[data-date="${focused}"]`)
        ?.focus();
      focusRequested.current = false;
    }
  }, [target, focused, month]);
  function apply(first: string, last: string) {
    subjectDateRange(first, last);
    onChange(first, last);
  }
  function enterCalendar(field: Endpoint) {
    const initial = field === "start" ? start || end : end;
    setFocused(initial);
    setMonth(parseDay(initial));
    setTarget(field);
    focusRequested.current = true;
  }
  function select(value: string, field: Endpoint) {
    setPreset(null);
    const first = field === "start" ? value : start;
    const last = field === "end" ? value : end;
    setCustomRange({ from: first, through: last });
    apply(first, last);
    setTarget(null);
  }
  function navigate(value: Date) {
    if (!validYear(value)) return;
    setMonth(value);
    setFocused(dateText(value));
  }
  function keyboard(event: KeyboardEvent<HTMLButtonElement>, value: Date) {
    const next = new Date(value);
    const key = event.key;
    if (key === "ArrowLeft") next.setDate(next.getDate() - 1);
    else if (key === "ArrowRight") next.setDate(next.getDate() + 1);
    else if (key === "ArrowUp") next.setDate(next.getDate() - 7);
    else if (key === "ArrowDown") next.setDate(next.getDate() + 7);
    else if (key === "Home") next.setDate(next.getDate() - next.getDay());
    else if (key === "End") next.setDate(next.getDate() + 6 - next.getDay());
    else if (key === "PageUp" || key === "PageDown") {
      next.setTime(
        moveMonth(
          value,
          (key === "PageUp" ? -1 : 1) * (event.shiftKey ? 12 : 1),
        ).getTime(),
      );
    } else return;
    event.preventDefault();
    if (validYear(next)) {
      focusRequested.current = true;
      navigate(next);
    }
  }
  const year = month.getFullYear(),
    monthIndex = month.getMonth();
  const label = `${year}年${monthIndex + 1}月`;
  const firstWeekday = localDay(year, monthIndex, 1).getDay();
  const days = localDay(year, monthIndex + 1, 0).getDate();
  const weeks = Math.ceil((firstWeekday + days) / 7);

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setTarget(null);
      }}
    >
      <div className="group relative inline-flex min-w-0">
        <PopoverTrigger asChild>
          <Button
            variant={active ? "secondary" : "ghost"}
            disabled={disabled}
            aria-label="时间"
            className={active ? "pr-9" : undefined}
          >
            <span className="max-w-[190px] truncate">{triggerLabel}</span>
            <ChevronDown
              className={
                active
                  ? "pointer-events-none absolute right-3 group-focus-within:opacity-0 group-hover:opacity-0 max-md:opacity-0"
                  : undefined
              }
            />
          </Button>
        </PopoverTrigger>
        {active ? (
          <Button
            variant="ghost"
            size="icon"
            disabled={disabled}
            aria-label="清空时间"
            className="absolute top-0 right-0 h-full w-9 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 max-md:opacity-100"
            onClick={() => {
              setPreset(null);
              setTarget(null);
              setCustomRange({ from: "", through: "" });
              apply("", "");
            }}
          >
            <X />
          </Button>
        ) : null}
      </div>
      <PopoverContent
        align="start"
        collisionPadding={16}
        aria-label="主体时间范围"
        className="unified-subject-popover subject-date-panel flex w-[min(300px,calc(100vw-32px))] flex-col gap-1 p-1"
      >
        <div className="flex items-center gap-2 p-2">
          {(["start", "end"] as const).map((field, index) => (
            <div key={field} className="flex min-w-0 flex-1 items-center gap-2">
              {index ? <span className="text-muted-foreground">-</span> : null}
              <Popover
                open={target === field}
                onOpenChange={(next) =>
                  next ? enterCalendar(field) : setTarget(null)
                }
              >
                <PopoverTrigger asChild>
                  <Button
                    variant="secondary"
                    disabled={disabled}
                    aria-label={field === "start" ? "开始日期" : "结束日期"}
                    className="min-w-0 flex-1 justify-between px-2 font-normal"
                  >
                    <span>
                      {field === "start"
                        ? start
                          ? shortDate(start)
                          : "开始日期"
                        : shortDate(end)}
                    </span>
                    <CalendarDays className="size-3.5 shrink-0" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="start"
                  collisionPadding={16}
                  aria-label={
                    field === "start" ? "选择开始日期" : "选择结束日期"
                  }
                  onOpenAutoFocus={(event) => {
                    event.preventDefault();
                    calendar.current
                      ?.querySelector<HTMLButtonElement>(
                        `[data-date="${focused}"]`,
                      )
                      ?.focus();
                  }}
                  className="unified-subject-popover w-[min(340px,calc(100vw-32px))] p-3"
                >
                  <div className="flex min-h-11 items-center justify-between gap-2">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="上一月"
                      disabled={year === 1 && monthIndex === 0}
                      onClick={() => navigate(moveMonth(parseDay(focused), -1))}
                    >
                      <ChevronLeft />
                    </Button>
                    <p aria-live="polite" className="text-sm">
                      {label}
                    </p>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="下一月"
                      disabled={year === 9999 && monthIndex === 11}
                      onClick={() => navigate(moveMonth(parseDay(focused), 1))}
                    >
                      <ChevronRight />
                    </Button>
                  </div>
                  <Table
                    ref={calendar}
                    role="grid"
                    aria-label={label}
                    className="w-full table-fixed border-separate border-spacing-0"
                  >
                    <TableHeader className="[&_tr]:border-0">
                      <TableRow className="border-0 hover:bg-transparent">
                        {["日", "一", "二", "三", "四", "五", "六"].map(
                          (text) => (
                            <TableHead
                              key={text}
                              scope="col"
                              className="h-6 px-0 text-center text-xs font-normal text-muted-foreground"
                            >
                              {text}
                            </TableHead>
                          ),
                        )}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {Array.from({ length: weeks }, (_, week) => (
                        <TableRow
                          key={week}
                          className="border-0 hover:bg-transparent"
                        >
                          {Array.from({ length: 7 }, (_, weekday) => {
                            const day = week * 7 + weekday - firstWeekday + 1;
                            if (day < 1 || day > days)
                              return (
                                <TableCell key={weekday} className="p-0" />
                              );
                            const value = localDay(year, monthIndex, day),
                              text = dateText(value);
                            const selected =
                              text === (field === "start" ? start : end);
                            const unavailable =
                              field === "start"
                                ? text > end
                                : Boolean(start && text < start);
                            return (
                              <TableCell
                                key={weekday}
                                aria-selected={selected}
                                className="p-0"
                              >
                                <Button
                                  type="button"
                                  variant={selected ? "default" : "ghost"}
                                  disabled={unavailable}
                                  tabIndex={focused === text ? 0 : -1}
                                  data-date={text}
                                  aria-label={`${year}年${monthIndex + 1}月${day}日`}
                                  aria-current={
                                    text === today ? "date" : undefined
                                  }
                                  className="h-11 w-full min-w-0 px-0"
                                  onFocus={() => setFocused(text)}
                                  onKeyDown={(event) => keyboard(event, value)}
                                  onClick={() => select(text, field)}
                                >
                                  {day}
                                </Button>
                              </TableCell>
                            );
                          })}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </PopoverContent>
              </Popover>
            </div>
          ))}
        </div>
        {presets.map((text, index) => (
          <Button
            key={text}
            variant={selectedPreset === text ? "secondary" : "ghost"}
            aria-pressed={selectedPreset === text}
            className="justify-between font-normal"
            disabled={disabled}
            onClick={() => {
              setTarget(null);
              if (!index) {
                setPreset(null);
                apply("", "");
                return;
              }
              const last = new Date();
              const first =
                index === 1
                  ? new Date(last)
                  : moveMonth(last, index === 2 ? -1 : -3);
              if (index === 1) first.setDate(first.getDate() - 6);
              const next = {
                label: text,
                from: dateText(first),
                through: dateText(last),
              };
              setPreset(next);
              apply(next.from, next.through);
            }}
          >
            {text}
            {selectedPreset === text ? <Check className="size-4" /> : null}
          </Button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

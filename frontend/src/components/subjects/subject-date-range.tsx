"use client";

import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
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
  const id = useId();
  const [open, setOpen] = useState(false);
  const [custom, setCustom] = useState(false);
  const [start, setStart] = useState(from);
  const [end, setEnd] = useState(through);
  const [target, setTarget] = useState<"start" | "end">("start");
  const [focused, setFocused] = useState(() => dateText(new Date()));
  const [month, setMonth] = useState(() => new Date());
  const [error, setError] = useState("");
  const calendar = useRef<HTMLTableElement>(null);
  const focusRequested = useRef(false);
  useEffect(() => {
    if (custom && focusRequested.current) {
      calendar.current
        ?.querySelector<HTMLButtonElement>(`[data-date="${focused}"]`)
        ?.focus();
      focusRequested.current = false;
    }
  }, [custom, focused, month]);
  function apply(first: string, last: string) {
    try {
      if (custom && (!first || !last))
        throw new Error("请选择开始日期和结束日期。");
      subjectDateRange(first, last);
      onChange(first, last);
      setOpen(false);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "请选择有效日期。");
    }
  }
  function enterCalendar() {
    const initial = start || end || dateText(new Date());
    setFocused(initial);
    setMonth(parseDay(initial));
    setCustom(true);
    focusRequested.current = true;
  }
  function select(value: string) {
    setFocused(value);
    if (target === "start") {
      setStart(value);
      setTarget("end");
    } else setEnd(value);
    setError("");
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
      const moved = moveMonth(
        value,
        (key === "PageUp" ? -1 : 1) * (event.shiftKey ? 12 : 1),
      );
      next.setTime(moved.getTime());
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
  const today = dateText(new Date());
  const rangeValid = start && end && start <= end;
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setStart(from);
          setEnd(through);
          setTarget("start");
          setError("");
          setCustom(false);
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant={from || through ? "secondary" : "ghost"}
          disabled={disabled}
          aria-label="时间"
        >
          时间
          <ChevronDown />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        collisionPadding={16}
        className={`unified-subject-popover subject-date-panel max-h-(--radix-popover-content-available-height) overflow-y-auto p-4 ${custom ? "w-[min(390px,calc(100vw-32px))]" : "w-[min(300px,calc(100vw-32px))]"}`}
        aria-label="主体时间范围"
      >
        {!custom ? (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-muted-foreground">时间</p>
            <Button variant="ghost" onClick={enterCalendar}>
              开始日期{from ? ` ${from}` : ""} — 结束日期
              {through ? ` ${through}` : ""}
            </Button>
            {["全部", "最近一周", "最近一个月", "最近三个月"].map(
              (text, index) => (
                <Button
                  key={text}
                  variant="ghost"
                  onClick={() => {
                    if (!index) {
                      apply("", "");
                      return;
                    }
                    const last = new Date();
                    const first =
                      index === 1
                        ? new Date(last)
                        : moveMonth(last, index === 2 ? -1 : -3);
                    if (index === 1) first.setDate(first.getDate() - 6);
                    apply(dateText(first), dateText(last));
                  }}
                >
                  {text}
                </Button>
              ),
            )}
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {(["start", "end"] as const).map((field) => (
              <Field key={field} className="gap-2">
                <FieldLabel
                  htmlFor={`${id}-${field}`}
                  className="text-xs text-muted-foreground"
                >
                  {field === "start" ? "开始日期" : "结束日期"}
                </FieldLabel>
                <Button
                  id={`${id}-${field}`}
                  variant="outline"
                  aria-label={field === "start" ? "开始日期" : "结束日期"}
                  aria-pressed={target === field}
                  aria-invalid={!!error}
                  aria-describedby={error ? `${id}-error` : `${id}-help`}
                  className="h-10 justify-start bg-background font-normal"
                  onClick={() => {
                    setTarget(field);
                    setError("");
                    const selected = field === "start" ? start : end;
                    if (selected) navigate(parseDay(selected));
                  }}
                >
                  {(field === "start" ? start : end) || (
                    <span className="text-muted-foreground">选择日期</span>
                  )}
                </Button>
              </Field>
            ))}
            <div className="flex min-h-11 items-center gap-2">
              <p
                id={`${id}-month`}
                aria-live="polite"
                className="mr-auto text-sm"
              >
                {label}
              </p>
              <Button
                variant="ghost"
                disabled={year === 1 && monthIndex === 0}
                onClick={() => navigate(moveMonth(parseDay(focused), -1))}
              >
                上一月
              </Button>
              <Button
                variant="ghost"
                disabled={year === 9999 && monthIndex === 11}
                onClick={() => navigate(moveMonth(parseDay(focused), 1))}
              >
                下一月
              </Button>
            </div>
            <Table
              ref={calendar}
              role="grid"
              aria-label={label}
              aria-multiselectable="true"
              className="w-[308px] max-w-full table-fixed border-separate border-spacing-0"
            >
              <TableHeader className="[&_tr]:border-0">
                <TableRow className="border-0 hover:bg-transparent">
                  {["日", "一", "二", "三", "四", "五", "六"].map((text) => (
                    <TableHead
                      key={text}
                      scope="col"
                      className="h-6 px-0 text-center text-xs font-normal text-muted-foreground"
                    >
                      {text}
                    </TableHead>
                  ))}
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
                        return <TableCell key={weekday} className="p-0" />;
                      const value = localDay(year, monthIndex, day),
                        text = dateText(value);
                      const endpoint = text === start || text === end;
                      const inRange =
                        !!rangeValid && text >= start && text <= end;
                      return (
                        <TableCell
                          key={weekday}
                          aria-selected={endpoint || inRange}
                          data-in-range={inRange || undefined}
                          className={inRange ? "bg-secondary p-0" : "p-0"}
                        >
                          <Button
                            type="button"
                            variant={endpoint ? "default" : "ghost"}
                            tabIndex={focused === text ? 0 : -1}
                            data-date={text}
                            aria-label={`${year}年${monthIndex + 1}月${day}日`}
                            aria-current={text === today ? "date" : undefined}
                            aria-description={
                              text === start
                                ? "开始日期"
                                : text === end
                                  ? "结束日期"
                                  : undefined
                            }
                            className="h-11 w-full min-w-0 px-0"
                            onFocus={() => setFocused(text)}
                            onKeyDown={(event) => keyboard(event, value)}
                            onClick={() => select(text)}
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
            <p
              id={`${id}-help`}
              aria-live="polite"
              className="text-xs text-muted-foreground"
            >
              {rangeValid
                ? `已选择 ${start} 至 ${end}，可应用时间范围。`
                : target === "start"
                  ? "先选择开始日期，再选择结束日期"
                  : "请选择结束日期"}
            </p>
            {error ? (
              <p
                id={`${id}-error`}
                role="alert"
                className="text-sm text-destructive"
              >
                {error}
              </p>
            ) : null}
            <div className="flex gap-2 pt-1">
              <Button
                variant="ghost"
                className="min-h-11"
                onClick={() => setOpen(false)}
              >
                取消
              </Button>
              <Button
                className="min-h-11 flex-1"
                onClick={() => apply(start, end)}
              >
                应用时间范围
              </Button>
            </div>
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}

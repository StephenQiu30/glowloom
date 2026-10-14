"use client";
import { useEffect, useState, useId } from "react";
import { Folder, Search } from "lucide-react";
import Link from "next/link";
import { listProjects } from "@/api/projects";
import {
  requireProjectList,
  type Project,
  projectErrorMessage,
} from "./project";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
} from "@/components/ui/input-group";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Alert, AlertDescription } from "@/components/ui/alert";
export function ProjectPicker({
  value,
  onChange,
  disabled = false,
  compact = false,
}: {
  value: Project | null;
  onChange: (p: Project) => void;
  disabled?: boolean;
  compact?: boolean;
}) {
  const fieldID = useId();
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<Project[]>([]),
    [search, setSearch] = useState(""),
    [query, setQuery] = useState(""),
    [cursor, setCursor] = useState<string | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    listProjects({ q: query, limit: 50 }, { signal: c.signal })
      .then(requireProjectList)
      .then((p) => {
        if (!c.signal.aborted) {
          setItems(p.items);
          setCursor(p.next_cursor);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(projectErrorMessage(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [query, retry]);
  async function more() {
    if (!cursor || loading) return;
    setLoading(true);
    try {
      const p = requireProjectList(
        await listProjects({ q: query, limit: 50, cursor }),
      );
      setItems((i) => [
        ...i,
        ...p.items.filter((v) => !i.some((a) => a.id === v.id)),
      ]);
      setCursor(p.next_cursor);
    } catch (e) {
      setError(projectErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }
  const controls = (
    <FieldGroup>
      <Field>
        <FieldLabel htmlFor={fieldID}>所属项目</FieldLabel>
        <Select
          value={value?.id ?? ""}
          onValueChange={(id) => {
            const project = items.find((item) => item.id === id);
            if (project) {
              onChange(project);
              setOpen(false);
            }
          }}
        >
          <SelectTrigger
            id={fieldID}
            className="w-full"
            aria-label="所属项目"
            disabled={disabled || loading}
          >
            <SelectValue placeholder={loading ? "正在读取项目…" : "选择项目"} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {value && !items.some((p) => p.id === value.id) && (
                <SelectItem value={value.id}>{value.name}</SelectItem>
              )}
              {items.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <Field>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setLoading(true);
            setQuery(search.trim());
            setRetry((n) => n + 1);
          }}
        >
          <InputGroup>
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              aria-label="查找项目"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="查找项目"
              disabled={disabled}
              maxLength={100}
            />
            <InputGroupAddon align="inline-end">
              <Button variant="ghost" disabled={disabled || loading}>
                查找
              </Button>
            </InputGroupAddon>
          </InputGroup>
        </form>
      </Field>
      {cursor && (
        <Button
          variant="ghost"
          disabled={disabled || loading}
          onClick={() => void more()}
        >
          更多项目
        </Button>
      )}
      {error ? (
        <Alert variant="danger">
          <AlertDescription>
            {error}
            <Button
              variant="ghost"
              onClick={() => {
                setLoading(true);
                setRetry((n) => n + 1);
              }}
            >
              重新读取
            </Button>
          </AlertDescription>
        </Alert>
      ) : !loading && items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          没有匹配的项目。<Link href="/projects">创建项目</Link>
        </p>
      ) : null}
    </FieldGroup>
  );
  if (compact)
    return (
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            className="max-w-64"
            aria-label="选择所属项目"
          >
            <Folder data-icon="inline-start" />
            <span className="truncate">{value?.name ?? "选择项目"}</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-80">
          {controls}
        </PopoverContent>
      </Popover>
    );
  return <div className="project-picker">{controls}</div>;
}

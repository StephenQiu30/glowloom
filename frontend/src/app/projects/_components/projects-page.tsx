"use client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FieldSet, Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { useRef, useState, useId } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Plus, Search } from "lucide-react";
import { ProductShell } from "@/components/layout/product-shell";
import { createProject } from "@/api/projects";
import {
  canvasHref,
  requireProject,
  projectErrorMessage,
} from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { ProjectList } from "./project-list";
export function ProjectsPage() {
  const formFieldId = useId();

  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [ratio, setRatio] = useState<"9:16" | "16:9" | "1:1">("16:9");
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const attempt = useRef<{ key: string; fingerprint: string } | null>(null);
  const busy = useRef(false);
  async function create() {
    if (busy.current) return;
    const body = { name: name.trim(), aspect_ratio: ratio };
    if (!body.name || Array.from(body.name).length > 50) {
      setError("请输入 1～50 字的画布名称。");
      return;
    }
    const fingerprint = JSON.stringify(body);
    if (attempt.current?.fingerprint !== fingerprint)
      attempt.current = { key: crypto.randomUUID(), fingerprint };
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const project = requireProject(
        await createProject(body, {
          headers: { "Idempotency-Key": attempt.current.key },
        }),
        true,
      );
      router.push(canvasHref(project));
    } catch (e) {
      setError(projectErrorMessage(e));
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  return (
    <ProductShell screen="projects">
      <nav className="workspace-tabs">
        <Link href="/assets">生成历史</Link>
        <Link href="/projects" aria-current="page">
          画布
        </Link>
      </nav>
      <div className="workspace-toolbar">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setQuery(search.trim());
          }}
          className="search-form"
        >
          <Search size={16} />
          <Input
            aria-label="搜索画布"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索画布"
          />
          <Button variant="ghost" type="submit">
            搜索
          </Button>
        </form>
        <span className="text-xs text-muted-foreground">最近修改</span>
        <Button className="ml-auto" onClick={() => setOpen(true)}>
          <Plus />
          新建画布
        </Button>
      </div>
      <ProjectList key={query} query={query} view="grid" />
      <Dialog
        open={open}
        onOpenChange={(v) => {
          if (!pending) setOpen(v);
        }}
      >
        <DialogContent>
          <DialogTitle>新建画布</DialogTitle>
          <DialogDescription>创建空白画布，开始你的创作。</DialogDescription>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void create();
            }}
          >
            <FieldGroup>
              <FieldSet disabled={pending}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                      <span>画布名称</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-1`}
                      autoFocus
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      placeholder="未命名画布"
                      maxLength={100}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                      <span>默认画幅</span>
                    </FieldLabel>
                    <Select
                      value={ratio}
                      onValueChange={(v) => setRatio(v as typeof ratio)}
                    >
                      <SelectTrigger
                        id={`${formFieldId}-field-2`}
                        aria-label="默认画幅"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {["16:9", "9:16", "1:1"].map((r) => (
                          <SelectItem key={r} value={r}>
                            {r}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                  {error && (
                    <Alert variant="destructive">
                      <AlertDescription>{error}</AlertDescription>
                    </Alert>
                  )}
                  <DialogFooter>
                    <Button
                      variant="outline"
                      type="button"
                      onClick={() => setOpen(false)}
                    >
                      取消
                    </Button>
                    <Button type="submit">
                      {pending ? "正在创建…" : "创建画布"}
                    </Button>
                  </DialogFooter>
                </FieldGroup>
              </FieldSet>
            </FieldGroup>
          </form>
        </DialogContent>
      </Dialog>
    </ProductShell>
  );
}

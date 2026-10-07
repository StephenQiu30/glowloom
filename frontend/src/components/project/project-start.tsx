"use client";

import { useState } from "react";
import { ArrowRight, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { InputGroup, InputGroupInput } from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { initialProjectDraft, type ProjectDraft } from "./creation";

export function ProjectStart({
  onStart,
}: {
  onStart: (draft: ProjectDraft, importScript: boolean) => void;
}) {
  const [draft, setDraft] = useState<ProjectDraft>({
    ...initialProjectDraft,
    aspect_ratio: "9:16",
  });
  const [error, setError] = useState("");
  function start(importScript: boolean) {
    const name = draft.name.trim();
    if (!name || Array.from(name).length > 50) {
      setError("请为项目起一个 1–50 字的名字。");
      document.getElementById("start-project-name")?.focus();
      return;
    }
    setError("");
    onStart({ ...draft, name }, importScript);
  }
  return (
    <section
      className="mx-auto flex w-full max-w-190 flex-col gap-6 pt-4 pb-14 md:pt-2 md:pb-16"
      aria-labelledby="start-project-title"
    >
      <header className="flex flex-col items-center gap-3 text-center">
        <h1
          id="start-project-title"
          className="text-[2rem] font-semibold tracking-tight"
        >
          从一份剧本开始
        </h1>
        <p className="text-sm leading-6 text-muted-foreground">
          创建你的故事，导入本地剧本，再进入画布逐镜创作。
        </p>
      </header>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          start(false);
        }}
        noValidate
      >
        <Card variant="composer" className="gap-3">
          <CardHeader className="sr-only">
            <CardTitle>创建项目</CardTitle>
            <CardDescription>
              设置名称、画幅与风格，确认后创建。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup className="gap-3">
              <Field data-invalid={Boolean(error)}>
                <FieldLabel htmlFor="start-project-name" className="sr-only">
                  项目名称
                </FieldLabel>
                <InputGroup variant="quiet">
                  <InputGroupInput
                    id="start-project-name"
                    placeholder="给新项目起个名字"
                    value={draft.name}
                    onChange={(event) => {
                      setDraft({ ...draft, name: event.target.value });
                      setError("");
                    }}
                    aria-invalid={Boolean(error)}
                    aria-describedby={error ? "start-project-error" : undefined}
                    autoComplete="off"
                  />
                </InputGroup>
                <FieldError id="start-project-error">{error}</FieldError>
              </Field>
              <Button
                type="button"
                variant="upload"
                onClick={() => start(true)}
                className="h-auto min-h-20 justify-start gap-4 px-4 py-4"
              >
                <Upload data-icon="inline-start" />
                <span className="flex min-w-0 flex-col items-start gap-1 whitespace-normal">
                  <span>导入本地剧本</span>
                  <span className="text-xs font-normal text-muted-foreground">
                    先创建项目，再选择文件导入
                  </span>
                </span>
              </Button>
            </FieldGroup>
          </CardContent>
          <CardFooter className="flex-wrap justify-between gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <ToggleGroup
                type="single"
                value={draft.aspect_ratio}
                onValueChange={(value) => {
                  if (value === "9:16" || value === "16:9")
                    setDraft({ ...draft, aspect_ratio: value });
                }}
                aria-label="画幅"
                size="sm"
              >
                <ToggleGroupItem value="9:16">9:16</ToggleGroupItem>
                <ToggleGroupItem value="16:9">16:9</ToggleGroupItem>
              </ToggleGroup>
              <Select
                value={draft.style_type}
                onValueChange={(value) =>
                  setDraft({
                    ...draft,
                    style_type:
                      value === "realistic" ? "realistic" : "stylized",
                    style_subtype: value === "realistic" ? "" : "anime_jp",
                  })
                }
              >
                <SelectTrigger size="sm" aria-label="项目风格">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="realistic">风格：写实</SelectItem>
                    <SelectItem value="stylized">风格：日系动漫</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => start(false)}
              >
                空白项目
              </Button>
            </div>
            <Button type="submit">
              创建项目
              <ArrowRight data-icon="inline-end" />
            </Button>
          </CardFooter>
        </Card>
      </form>
    </section>
  );
}

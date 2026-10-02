"use client";
import { useId, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldError,
  FieldDescription,
} from "@/components/ui/field";
import { LibrarySelect } from "./library-select";
import {
  categoryLabels,
  folderStyles,
  folderThemes,
  folderStyleLabels,
  folderThemeLabels,
  folderAncestry,
  canReparentLibraryFolder,
  libraryFolderMoveTargets,
  libraryCategories,
  libraryMetadataSchema,
  libraryText,
  libraryUUID,
  type LibraryFolder,
  type LibraryMetadata,
  type LibraryScope,
} from "./library-model";
const folderForm = z.object({
  name: libraryText(60, true).refine(
    (name) => name === name.trim(),
    "请去除名称两侧空白。",
  ),
  parent_id: libraryUUID.nullable(),
  style: z.string(),
  theme: z.string(),
});
export function FolderForm({
  scope,
  folders,
  current,
  draft,
  locked,
  onDirty,
  onSubmit,
}: {
  scope: LibraryScope;
  folders: LibraryFolder[];
  current?: LibraryFolder;
  draft?: z.infer<typeof folderForm>;
  locked: boolean;
  onDirty: () => void;
  onSubmit: (folder: z.infer<typeof folderForm>) => void;
}) {
  const id = useId(),
    form = useForm({
      resolver: zodResolver(folderForm),
      defaultValues: draft ?? {
        name: current?.name ?? "",
        parent_id: current?.parent_id ?? null,
        style: scope.kind === "personal" ? "" : (current?.style ?? "paper"),
        theme: scope.kind === "personal" ? "" : (current?.theme ?? "pearl"),
      },
    });
  const parentOptions = [
    { value: "root", label: "根目录" },
    ...libraryFolderMoveTargets(folders, current?.id)
      .filter(
        ({ path }) =>
          !current || !path.some((ancestor) => ancestor.id === current.id),
      )
      .map(({ folder, path, allowed }) => ({
        value: folder.id,
        label: path.map((ancestor) => ancestor.name).join(" / "),
        disabled: !allowed,
      })),
  ];
  return (
    <form
      onChange={onDirty}
      onSubmit={form.handleSubmit((folder) => {
        if (scope.kind === "personal" && Array.from(folder.name).length > 40) {
          form.setError("name", { message: "个人目录最多40个Unicode字符。" });
          return;
        }
        if (
          scope.kind === "project" &&
          !canReparentLibraryFolder(folders, current?.id, folder.parent_id)
        ) {
          form.setError("parent_id", {
            message: "目录移动不能形成环或超过8层。",
          });
          return;
        }
        onSubmit(folder);
      })}
    >
      <fieldset disabled={locked} className="flex flex-col gap-4">
        <FieldGroup>
          <Field data-invalid={Boolean(form.formState.errors.name)}>
            <FieldLabel htmlFor={`${id}-name`}>目录名称</FieldLabel>
            <Input
              id={`${id}-name`}
              {...form.register("name")}
              aria-invalid={Boolean(form.formState.errors.name)}
            />
            <FieldDescription>
              {scope.kind === "personal"
                ? "扁平分类，名称1..40个Unicode字符。"
                : "最多8层，名称1..60个Unicode字符。"}
            </FieldDescription>
            <FieldError errors={[form.formState.errors.name]} />
          </Field>
          {scope.kind === "project" && (
            <>
              <Field>
                <FieldLabel htmlFor={`${id}-parent`}>上级目录</FieldLabel>
                <Controller
                  control={form.control}
                  name="parent_id"
                  render={({ field }) => (
                    <LibrarySelect
                      id={`${id}-parent`}
                      label="上级目录"
                      value={field.value ?? "root"}
                      options={parentOptions}
                      disabled={locked}
                      onChange={(value) => {
                        onDirty();
                        field.onChange(value === "root" ? null : value);
                      }}
                    />
                  )}
                />
                <FieldError errors={[form.formState.errors.parent_id]} />
              </Field>
              <Field>
                <FieldLabel>目录样式</FieldLabel>
                <Controller
                  control={form.control}
                  name="style"
                  render={({ field }) => (
                    <LibrarySelect
                      label="目录样式"
                      value={field.value}
                      options={folderStyles.map((value) => ({
                        value,
                        label: folderStyleLabels[value],
                      }))}
                      disabled={locked}
                      onChange={(value) => {
                        onDirty();
                        field.onChange(value);
                      }}
                    />
                  )}
                />
              </Field>
              <Field>
                <FieldLabel>目录主题</FieldLabel>
                <Controller
                  control={form.control}
                  name="theme"
                  render={({ field }) => (
                    <LibrarySelect
                      label="目录主题"
                      value={field.value}
                      options={folderThemes.map((value) => ({
                        value,
                        label: folderThemeLabels[value],
                      }))}
                      disabled={locked}
                      onChange={(value) => {
                        onDirty();
                        field.onChange(value);
                      }}
                    />
                  )}
                />
              </Field>
            </>
          )}
        </FieldGroup>
        <Button type="submit" disabled={locked}>
          保存目录
        </Button>
      </fieldset>
    </form>
  );
}
export function MetadataForm({
  scope,
  folders,
  metadata,
  text,
  locked,
  onDirty,
  onSubmit,
}: {
  scope: LibraryScope;
  folders: LibraryFolder[];
  metadata?: LibraryMetadata;
  text: boolean;
  locked: boolean;
  onDirty: () => void;
  onSubmit: (metadata: LibraryMetadata) => void;
}) {
  const [tagsText, setTagsText] = useState(metadata?.tags.join(", ") ?? "");
  const id = useId(),
    form = useForm({
      resolver: zodResolver(libraryMetadataSchema),
      defaultValues: metadata ?? {
        title: "",
        category: "material",
        tags: [],
        folder_id: null,
        source_label: "",
        note: "",
        favorite: false,
        ...(text ? { plain_text: "" } : {}),
      },
    });
  return (
    <form onChange={onDirty} onSubmit={form.handleSubmit(onSubmit)}>
      <fieldset disabled={locked} className="flex flex-col gap-4">
        <FieldGroup>
          <Field data-invalid={Boolean(form.formState.errors.title)}>
            <FieldLabel htmlFor={`${id}-title`}>素材标题</FieldLabel>
            <Input
              id={`${id}-title`}
              {...form.register("title")}
              aria-invalid={Boolean(form.formState.errors.title)}
            />
            <FieldError errors={[form.formState.errors.title]} />
          </Field>
          <Field>
            <FieldLabel>业务分类</FieldLabel>
            <Controller
              control={form.control}
              name="category"
              render={({ field }) => (
                <LibrarySelect
                  label="业务分类"
                  value={field.value}
                  disabled={locked}
                  options={libraryCategories.map((value) => ({
                    value,
                    label: categoryLabels[value],
                  }))}
                  onChange={(value) => {
                    onDirty();
                    field.onChange(value);
                  }}
                />
              )}
            />
          </Field>
          <Field>
            <FieldLabel>所属目录</FieldLabel>
            <Controller
              control={form.control}
              name="folder_id"
              render={({ field }) => (
                <LibrarySelect
                  label="所属目录"
                  value={field.value ?? "root"}
                  disabled={locked}
                  options={[
                    { value: "root", label: "未分类" },
                    ...folders.map((folder) => ({
                      value: folder.id,
                      label: folderAncestry(folders, folder.id)
                        .map((ancestor) => ancestor.name)
                        .join(" / "),
                    })),
                  ]}
                  onChange={(value) => {
                    onDirty();
                    field.onChange(value === "root" ? null : value);
                  }}
                />
              )}
            />
          </Field>
          <Field data-invalid={Boolean(form.formState.errors.tags)}>
            <FieldLabel htmlFor={`${id}-tags`}>标签</FieldLabel>
            <Controller
              control={form.control}
              name="tags"
              render={({ field }) => (
                <Input
                  id={`${id}-tags`}
                  value={tagsText}
                  onChange={(event) => {
                    setTagsText(event.target.value);
                    field.onChange(
                      event.target.value
                        .split(/[,，]/)
                        .map((tag) => tag.trim())
                        .filter(Boolean),
                    );
                  }}
                  aria-invalid={Boolean(form.formState.errors.tags)}
                />
              )}
            />
            <FieldDescription>
              逗号分隔，最多32项，每项64个字符，不能重复。
            </FieldDescription>
            <FieldError errors={[form.formState.errors.tags]} />
          </Field>
          <Field data-invalid={Boolean(form.formState.errors.source_label)}>
            <FieldLabel htmlFor={`${id}-source`}>来源说明</FieldLabel>
            <Input id={`${id}-source`} {...form.register("source_label")} />
            <FieldDescription>此说明只用于分类与检索。</FieldDescription>
            <FieldError errors={[form.formState.errors.source_label]} />
          </Field>
          <Field data-invalid={Boolean(form.formState.errors.note)}>
            <FieldLabel htmlFor={`${id}-note`}>备注</FieldLabel>
            <Textarea id={`${id}-note`} {...form.register("note")} />
            <FieldError errors={[form.formState.errors.note]} />
          </Field>
          {text && (
            <Field data-invalid={Boolean(form.formState.errors.plain_text)}>
              <FieldLabel htmlFor={`${id}-text`}>文本内容</FieldLabel>
              <Textarea
                id={`${id}-text`}
                {...form.register("plain_text")}
                className="min-h-48"
              />
              <FieldDescription>
                纯文本，完整保留空白与换行，最多64 KiB。
              </FieldDescription>
              <FieldError errors={[form.formState.errors.plain_text]} />
            </Field>
          )}
          {scope.kind === "personal" && (
            <Field orientation="horizontal">
              <Controller
                control={form.control}
                name="favorite"
                render={({ field }) => (
                  <Checkbox
                    id={`${id}-favorite`}
                    checked={field.value}
                    disabled={locked}
                    onCheckedChange={(value) => {
                      onDirty();
                      field.onChange(value === true);
                    }}
                  />
                )}
              />
              <FieldLabel htmlFor={`${id}-favorite`}>收藏此素材</FieldLabel>
            </Field>
          )}
        </FieldGroup>
        <Button type="submit" disabled={locked}>
          保存素材描述
        </Button>
      </fieldset>
    </form>
  );
}

import { useId } from "react";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

/** A new publication requires the administrator to enter its own review evidence. */
export function PriceEvidenceFields({
  price,
}: {
  price?: { source_url?: string; reviewed_at?: string };
}) {
  const id = useId();
  return (
    <>
      {price && (
        <p className="text-xs break-words text-muted-foreground">
          上一版本来源：{price.source_url ?? "未记录"}；核验：
          {price.reviewed_at ?? "未记录"}
        </p>
      )}
      <Field>
        <FieldLabel htmlFor={`${id}-source`}>价格来源链接</FieldLabel>
        <Input
          id={`${id}-source`}
          name="source_url"
          type="url"
          pattern="https://.*"
          required
          placeholder="https://供应商官方价格说明"
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-reviewed`}>核验时间</FieldLabel>
        <Input
          id={`${id}-reviewed`}
          name="reviewed_at"
          type="datetime-local"
          required
        />
      </Field>
      <p className="text-xs text-muted-foreground">
        填写本次实际核验的一手价格说明和时间。链接不能包含凭据或查询参数。
      </p>
    </>
  );
}

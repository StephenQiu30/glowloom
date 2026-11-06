import { useId } from "react";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

/** A new publication requires the administrator to enter its own review evidence. */
export function PriceEvidenceFields({
  price,
  compact = false,
}: {
  price?: Pick<API.AdminPrice, "source_url" | "reviewed_at">;
  compact?: boolean;
}) {
  const id = useId();
  return (
    <>
      {price && (
        <Collapsible>
          <CollapsibleTrigger asChild>
            <Button type="button" variant="ghost" size="sm">
              上次核验信息
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="pt-2 text-xs break-words text-muted-foreground">
            上一版本来源：{price.source_url ?? "未记录"}；核验：
            {price.reviewed_at ?? "未记录"}
          </CollapsibleContent>
        </Collapsible>
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
      {!compact && (
        <p className="text-xs text-muted-foreground">
          使用本次核验的官方价格说明。
        </p>
      )}
    </>
  );
}

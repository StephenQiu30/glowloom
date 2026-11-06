import { useId } from "react";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

export function PriceEvidenceFields({
  sourceURL,
}: {
  sourceURL?: string | null;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>官方价格来源</FieldLabel>
      <Input
        id={id}
        name="source_url"
        type="url"
        pattern="https://.*"
        defaultValue={sourceURL ?? ""}
        required
      />
    </Field>
  );
}

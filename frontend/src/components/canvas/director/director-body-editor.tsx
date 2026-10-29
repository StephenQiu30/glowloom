"use client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FieldSet, FieldLegend } from "@/components/ui/field";
import { Label } from "@/components/ui/label";
import type { DirectorObject } from "./director-model";
import {
  DIRECTOR_BODY_FIELDS,
  DIRECTOR_BODY_MIN,
  DIRECTOR_BODY_MAX,
  directorBodyProfile,
  supportsDirectorBody,
} from "./director-body-profile";

export function DirectorBodyEditor({
  object,
  disabled,
  onChange,
}: {
  object: DirectorObject;
  disabled: boolean;
  onChange: (patch: Partial<DirectorObject>) => void;
}) {
  if (!supportsDirectorBody(object)) return null;
  const profile = directorBodyProfile(
    object.builtin_actor!,
    object.body_profile,
  );
  const readOnly = disabled || object.locked === true;
  const update = (body_profile: DirectorObject["body_profile"]) => {
    if (!readOnly)
      onChange({
        body_profile,
        frozen_nodes: undefined,
        frozen_bones: undefined,
      });
  };
  return (
    <FieldSet className="gap-2" disabled={readOnly}>
      <FieldLegend variant="label">身体参数</FieldLegend>
      {DIRECTOR_BODY_FIELDS.map(([key, label]) => (
        <Label key={key} className="director-field">
          {label}
          <Input
            key={`${object.id}:${profile[key]}`}
            aria-label={label}
            type="number"
            defaultValue={profile[key]}
            min={DIRECTOR_BODY_MIN}
            max={DIRECTOR_BODY_MAX}
            step="0.01"
            disabled={readOnly}
            onBlur={(event) => {
              const value = Number(event.target.value);
              if (
                !readOnly &&
                event.target.value.trim() &&
                Number.isFinite(value) &&
                value >= DIRECTOR_BODY_MIN &&
                value <= DIRECTOR_BODY_MAX &&
                value !== profile[key]
              )
                update({ ...object.body_profile, [key]: value });
              else event.target.value = String(profile[key]);
            }}
            onKeyDown={(event) => {
              event.stopPropagation();
              if (event.key === "Enter") event.currentTarget.blur();
            }}
          />
        </Label>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={
          readOnly ||
          !object.body_profile ||
          Object.keys(object.body_profile).length === 0
        }
        onClick={() => update(undefined)}
      >
        重置身体参数
      </Button>
    </FieldSet>
  );
}

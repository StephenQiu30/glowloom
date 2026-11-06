"use client";

import {
  AudioLines,
  Clapperboard,
  Image,
  Music2,
  PersonStanding,
  ScanFace,
  Sparkles,
} from "lucide-react";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { creationTypes, type CreationType } from "./creation-types";

const icons = {
  agent: Sparkles,
  image: Image,
  video: Clapperboard,
  music: Music2,
  audio: AudioLines,
  avatar: ScanFace,
  motion: PersonStanding,
};
export function CreationTypeSelect({
  value,
  disabled,
  onChange,
}: {
  value: CreationType;
  disabled?: boolean;
  onChange: (type: CreationType) => void;
}) {
  const Icon = icons[value];
  return (
    <Select
      value={value}
      disabled={disabled}
      onValueChange={(value) => onChange(value as CreationType)}
    >
      <SelectTrigger
        aria-label="创作类型"
        variant="quiet"
        className="w-auto text-[var(--navigation-accent)] max-md:min-h-11 [&_svg]:text-[var(--navigation-accent)]"
      >
        <Icon aria-hidden="true" />
        <SelectValue>
          {creationTypes.find((item) => item.id === value)?.label}
        </SelectValue>
      </SelectTrigger>
      <SelectContent side="top" sideOffset={8}>
        <SelectGroup>
          {creationTypes.map((item) => {
            const ItemIcon = icons[item.id];
            return (
              <SelectItem key={item.id} value={item.id}>
                <ItemIcon aria-hidden="true" />
                {item.label}
              </SelectItem>
            );
          })}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

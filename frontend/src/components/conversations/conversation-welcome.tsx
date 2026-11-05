import Image from "next/image";
import { WandSparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { ConversationSkillPin } from "./conversation-context";
import { isSameSkillVersion, isVersionedSkill } from "./conversation-state";

const covers = [
  ["叙事短片导演分镜", "/design/reference/moonlight-whale.png"],
  ["系列套图生成", "/design/reference/amber-perfume.png"],
  ["爆款电商短视频题材创意", "/design/reference/cobalt-headphones.png"],
] as const;

function recommend(
  skills: API.SkillVersion[],
  selected: ConversationSkillPin[],
) {
  const available = skills
    .filter(isVersionedSkill)
    .filter((skill) => skill.enabled && skill.name?.trim());
  const byID = new Map<string, (typeof available)[number]>();
  for (const skill of available) {
    if (
      !byID.has(skill.skill_id) ||
      selected.some((pin) => isSameSkillVersion(pin, skill))
    ) {
      byID.set(skill.skill_id, skill);
    }
  }
  const entries = [...byID.values()];
  const featured = covers.flatMap(([name]) => {
    const skill = entries.find((item) => item.name === name);
    return skill ? [skill] : [];
  });
  return [
    ...featured,
    ...entries.filter((skill) => !featured.includes(skill)),
  ].slice(0, 3);
}

export function ConversationWelcome({
  skills,
  selected,
  disabled,
  onSelect,
}: {
  skills: API.SkillVersion[];
  selected: ConversationSkillPin[];
  disabled: boolean;
  onSelect: (skill: ConversationSkillPin) => void;
}) {
  const recommended = recommend(skills, selected);
  return (
    <section
      className="min-h-0 flex-1 overflow-y-auto"
      aria-labelledby="conversation-welcome-title"
    >
      <div className="mx-auto flex w-full max-w-[800px] flex-col gap-7 pt-8 pb-4 md:gap-11 md:pt-[clamp(2rem,calc(14dvh-3.625rem),5.5rem)]">
        <h1
          id="conversation-welcome-title"
          className="text-center text-2xl leading-9 font-normal"
        >
          你好，想创作什么？
        </h1>
        {recommended.length > 0 && (
          <ul
            className="grid snap-x snap-mandatory auto-cols-[min(86%,16.5rem)] grid-flow-col gap-2 overflow-x-auto pb-2 md:grid-flow-row md:grid-cols-[repeat(auto-fill,minmax(min(100%,14rem),1fr))] md:overflow-visible"
            aria-label="推荐技能"
          >
            {recommended.map((skill) => {
              const name = skill.name ?? "";
              const cover = covers.find(([title]) => title === name)?.[1];
              const chosen = selected.some((pin) =>
                isSameSkillVersion(pin, skill),
              );
              const action = chosen ? "已选用" : "试一试";
              return (
                <li key={skill.skill_id} className="min-w-0 snap-start">
                  <Card size="sm" className="h-full gap-0 bg-muted p-0 ring-0">
                    <div className="relative isolate flex h-[188px] flex-col justify-end bg-muted">
                      {cover ? (
                        <Image
                          src={cover}
                          alt=""
                          fill
                          sizes="264px"
                          className="object-cover"
                        />
                      ) : (
                        <WandSparkles
                          aria-hidden="true"
                          className="absolute top-10 left-1/2 size-10 -translate-x-1/2 text-muted-foreground"
                        />
                      )}
                      <CardHeader className="relative flex flex-col items-start gap-1 bg-linear-to-t from-black/70 to-transparent px-3.5 pt-7 pb-3 text-white">
                        <Badge
                          variant="ghost"
                          className="h-auto px-0 text-[10px] font-normal"
                        >
                          {cover ? "热门技能" : "可用技能"}
                        </Badge>
                        <div className="flex w-full items-center justify-between gap-2">
                          <CardTitle title={name} className="truncate">
                            {name}
                          </CardTitle>
                          <Button
                            type="button"
                            variant="outline"
                            size="xs"
                            className="border-white/35 bg-black/50 text-white hover:bg-black/75 hover:text-white pointer-coarse:min-h-11"
                            disabled={disabled}
                            aria-label={`${action}：${name}`}
                            onClick={() => onSelect(skill)}
                          >
                            {action}
                          </Button>
                        </div>
                      </CardHeader>
                    </div>
                    <div className="min-h-[68px] px-3.5 py-3">
                      <CardDescription
                        title={skill.description}
                        className="line-clamp-2 text-xs leading-5"
                      >
                        {skill.description}
                      </CardDescription>
                    </div>
                  </Card>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}

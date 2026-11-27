import Image from "next/image";
import { cn } from "cn";
import { ArrowUpRight, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { ConversationSkillPin } from "./conversation-context";
import { isSameSkillVersion, isVersionedSkill } from "./conversation-state";
import styles from "./conversation-welcome.module.css";

const starters = [
  {
    name: "系列套图生成",
    cover: "/design/reference/amber-perfume.png",
    description: "围绕同一角色或产品，制作风格一致的系列图片。",
    prompt:
      "请根据我提供的产品或参考图，设计一组风格统一的图片方案，说明共同的视觉风格及每张图的构图和提示词。先给出方案，等待我确认。",
  },
  {
    name: "爆款电商短视频题材创意",
    cover: "/design/reference/cobalt-headphones.png",
    description: "整理商品卖点，生成短视频创意与拍摄提纲。",
    prompt:
      "请根据我提供的商品与目标受众，提出三组电商短视频创意，分别给出开场钩子、主卖点和镜头结构。若信息不足，请先向我提问。",
  },
] as const;

export function ConversationWelcome({
  skills,
  showStarters = true,
  selected,
  disabled,
  onSelect,
}: {
  skills: API.SkillVersion[];
  showStarters?: boolean;
  selected: ConversationSkillPin[];
  disabled: boolean;
  onSelect: (prompt: string, skill?: ConversationSkillPin) => void;
}) {
  const available = skills
    .filter(isVersionedSkill)
    .filter((skill) => skill.enabled);
  return (
    <section
      className="min-h-0 flex-1 overflow-y-auto"
      aria-labelledby="conversation-welcome-title"
    >
      <div
        className={cn(
          styles.content,
          "mx-auto flex w-full max-w-[800px] flex-col",
          showStarters
            ? styles.withStarters
            : "md:pt-[clamp(4rem,calc(30dvh-2.5rem),18rem)]",
        )}
      >
        <h1
          id="conversation-welcome-title"
          className={cn(styles.heading, "text-center font-medium")}
        >
          你好，想创作什么？
        </h1>
        {showStarters && (
          <ul
            className={cn(styles.starters, "grid grid-cols-2")}
            aria-label="推荐技能"
          >
            {starters.map(({ name, cover, description, prompt }) => {
              const matches = available.filter((skill) => skill.name === name);
              const chosen = matches.find((skill) =>
                selected.some((pin) => isSameSkillVersion(pin, skill)),
              );
              const skill = chosen ?? matches[0];
              const action = chosen ? "已选用" : "试一试";
              return (
                <li key={name} className="min-w-0">
                  <Card variant="media" className="h-full gap-0">
                    <div
                      className={cn(
                        styles.cover,
                        "relative overflow-hidden rounded-xl bg-muted",
                      )}
                    >
                      <Image
                        src={cover}
                        alt=""
                        fill
                        sizes="(min-width: 1060px) 394px, (min-width: 768px) 33vw, 46vw"
                        className="object-cover"
                      />
                    </div>
                    <CardHeader className={cn(styles.details, "px-1 pt-3")}>
                      <CardTitle>{name}</CardTitle>
                      <CardDescription className="line-clamp-2">
                        {description}
                      </CardDescription>
                      <CardAction className={styles.action}>
                        <Button
                          type="button"
                          variant="ghost"
                          size="toolbar-adaptive"
                          disabled={disabled}
                          aria-label={`${action}：${name}`}
                          aria-pressed={skill ? Boolean(chosen) : undefined}
                          title={action}
                          onClick={() => onSelect(prompt, skill)}
                        >
                          <span className="max-md:sr-only">{action}</span>
                          {chosen ? (
                            <Check data-icon="inline-end" />
                          ) : (
                            <ArrowUpRight data-icon="inline-end" />
                          )}
                        </Button>
                      </CardAction>
                    </CardHeader>
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

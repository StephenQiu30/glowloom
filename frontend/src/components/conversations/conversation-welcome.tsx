import Image from "next/image";
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

const starters = [
  {
    name: "叙事短片导演分镜",
    cover: "/design/reference/moonlight-whale.png",
    description: "从故事构思到镜头设计，逐步完成分镜与创作提示词。",
    prompt:
      "请把我的故事构思拆成短片分镜，逐镜列出画面、景别、运镜、时长和创作提示词。若缺少故事背景，请先向我提问。",
  },
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
    description: "整理商品卖点，生成短视频创意与分镜方案。",
    prompt:
      "请根据我提供的商品与目标受众，提出三组电商短视频创意，分别给出开场钩子、主卖点和镜头结构。若信息不足，请先向我提问。",
  },
] as const;

export function ConversationWelcome({
  skills,
  selected,
  disabled,
  onSelect,
}: {
  skills: API.SkillVersion[];
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
      <div className="mx-auto flex w-full max-w-[800px] flex-col gap-7 pt-8 pb-4 md:gap-10 md:pt-[clamp(2rem,calc(18dvh-3.375rem),9rem)]">
        <h1
          id="conversation-welcome-title"
          className="text-center text-2xl leading-8 font-normal"
        >
          你好，想创作什么？
        </h1>
        <ul
          className="grid snap-x snap-mandatory auto-cols-[min(86%,16.5rem)] grid-flow-col gap-2 overflow-x-auto pb-2 md:grid-flow-row md:grid-cols-[repeat(auto-fill,minmax(min(100%,14rem),1fr))] md:overflow-visible"
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
              <li key={name} className="min-w-0 snap-start">
                <Card size="sm" className="h-full gap-0 p-0 ring-0">
                  <div className="relative isolate flex h-[188px] flex-col justify-end bg-muted">
                    <Image
                      src={cover}
                      alt=""
                      fill
                      sizes="264px"
                      className="object-cover"
                    />
                    <CardHeader className="relative flex flex-col items-start gap-1 bg-linear-to-t from-black/70 to-transparent px-3.5 pt-7 pb-3 text-white">
                      <Badge
                        variant="ghost"
                        className="h-auto px-0 text-[10px] font-normal"
                      >
                        {skill ? "热门技能" : "创作灵感"}
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
                          onClick={() => onSelect(prompt, skill)}
                        >
                          {action}
                        </Button>
                      </div>
                    </CardHeader>
                  </div>
                  <div className="min-h-[68px] px-3.5 py-3">
                    <CardDescription
                      title={skill?.description || description}
                      className="line-clamp-2 text-xs leading-5"
                    >
                      {skill?.description || description}
                    </CardDescription>
                  </div>
                </Card>
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}

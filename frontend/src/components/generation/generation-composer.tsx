"use client";
import { useState } from "react";
import {
  AtSign,
  ImageIcon,
  Plus,
  SlidersHorizontal,
  VideoIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup } from "@/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { GenerationDialog, type GenerationSeed } from "./generation-dialog";
export function GenerationComposer() {
  const [prompt, setPrompt] = useState("");
  const [capability, setCapability] = useState<"image" | "video">("image");
  const [seed, setSeed] = useState<GenerationSeed | null>(null);
  function configure(pickSubject = false) {
    setSeed({ prompt, capability, pickSubject });
  }
  return (
    <>
      <form
        className="asset-composer"
        onSubmit={(e) => {
          e.preventDefault();
          if (prompt.trim()) configure();
        }}
      >
        <FieldGroup>
          <Field>
            <InputGroup variant="composer" className="generation-composer">
              <InputGroupAddon align="block-start">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => configure()}
                >
                  <Plus data-icon="inline-start" />
                  参考素材
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => configure(true)}
                >
                  <AtSign data-icon="inline-start" />
                  主体
                </Button>
              </InputGroupAddon>
              <InputGroupTextarea
                aria-label="资产创作提示词"
                placeholder="描述你想创作的画面，添加参考素材与主体"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                maxLength={10000}
              />
              <InputGroupAddon align="block-end">
                <Select
                  value={capability}
                  onValueChange={(v) => setCapability(v as "image" | "video")}
                >
                  <SelectTrigger variant="quiet" aria-label="资产生成类型">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="image">
                        <ImageIcon />
                        图片生成
                      </SelectItem>
                      <SelectItem value="video">
                        <VideoIcon />
                        视频生成
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => configure()}
                >
                  <SlidersHorizontal data-icon="inline-start" />
                  选择模型与参数
                </Button>
                <Button
                  type="submit"
                  className="ml-auto"
                  size="lg"
                  disabled={!prompt.trim()}
                >
                  生成
                </Button>
              </InputGroupAddon>
            </InputGroup>
          </Field>
        </FieldGroup>
      </form>
      {seed && <GenerationDialog seed={seed} onClose={() => setSeed(null)} />}
    </>
  );
}

"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { stageLabels } from "@/components/creation/proposal-content";
import { proposalTitle } from "@/components/creation/proposal-context";

export function ProposalNavigation({
  proposals,
  selectedId,
  onSelect,
}: {
  proposals: API.CreationProposal[];
  selectedId?: string;
  onSelect: (id: string) => void;
}) {
  const [query, setQuery] = useState("");
  const filtered = proposals.filter((proposal) =>
    `${stageLabels[proposal.stage]} ${proposalTitle(proposal, proposals)}`
      .toLocaleLowerCase()
      .includes(query.trim().toLocaleLowerCase()),
  );

  return (
    <nav
      aria-label="创作内容目录"
      className="flex flex-col min-w-0 gap-5 lg:sticky lg:top-6 lg:max-h-[85dvh] lg:overflow-y-auto lg:pr-3"
    >
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold">创作内容</h2>
        <Input
          aria-label="查找剧集或场景"
          placeholder="查找剧集或场景"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </div>
      {Object.entries(stageLabels).map(([stage, label]) => {
        const items = filtered.filter((item) => item.stage === stage);
        if (query && !items.length) return null;
        return (
          <section className="flex flex-col gap-2" key={stage}>
            <h3 className="text-xs font-medium text-muted-foreground">
              {label} · {items.length}
            </h3>
            {items.length ? (
              <ul className="flex flex-col gap-1">
                {items.map((proposal) => (
                  <li key={proposal.id}>
                    <Button
                      variant="ghost"
                      type="button"
                      aria-current={selectedId === proposal.id ? "true" : undefined}
                      className="h-auto w-full flex-col items-start gap-1 px-3 py-3 text-left whitespace-normal"
                      onClick={() => onSelect(proposal.id)}
                    >
                      <span className="block break-words font-medium">
                        {proposalTitle(proposal, proposals)}
                      </span>
                      <span className="mt-1 block text-xs text-muted-foreground">
                        {proposal.status === "accepted" ? "已采纳" : "待审阅"}
                      </span>
                    </Button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="px-3 py-2 text-xs text-muted-foreground">
                尚无内容，完成前序确认后生成。
              </p>
            )}
          </section>
        );
      })}
      {query && !filtered.length && (
        <p role="status" className="text-sm text-muted-foreground">
          没有找到匹配的剧集或场景，请更换关键词。
        </p>
      )}
    </nav>
  );
}

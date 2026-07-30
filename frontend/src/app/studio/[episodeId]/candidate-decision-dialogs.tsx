"use client";

import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

import { proposalDescription, proposalTitle } from "./episode-studio-model";

type CandidateDecision = API.CandidateDecisionRequest["decision"];
type CandidateProposal = Extract<
  CandidateDecision,
  { action: "accept_with_changes" }
>["proposal"];

type DecisionHandler = (
  candidate: API.ExtractionCandidateResponse,
  decision: CandidateDecision,
) => Promise<boolean>;

function editableProposalFields(candidate: API.ExtractionCandidateResponse) {
  switch (candidate.proposal.kind) {
    case "scene":
      return { title: candidate.proposal.heading, description: candidate.proposal.summary };
    case "dialogue":
      return {
        title: candidate.proposal.speaker_candidate,
        description: candidate.proposal.text,
      };
    case "asset":
      return { title: candidate.proposal.name, description: candidate.proposal.description };
    case "shot":
      return { title: candidate.proposal.title, description: candidate.proposal.purpose };
    case "continuity":
      return { title: candidate.proposal.issue, description: candidate.proposal.suggestion };
  }
}

function editedProposal(
  candidate: API.ExtractionCandidateResponse,
  title: string,
  description: string,
): CandidateProposal {
  switch (candidate.proposal.kind) {
    case "scene":
      return { ...candidate.proposal, heading: title, summary: description };
    case "dialogue":
      return {
        ...candidate.proposal,
        speaker_candidate: title,
        text: description,
      };
    case "asset":
      return { ...candidate.proposal, name: title, description };
    case "shot":
      return { ...candidate.proposal, title, purpose: description };
    case "continuity":
      return { ...candidate.proposal, issue: title, suggestion: description };
  }
}

export function CandidateEditDialog({
  busy,
  candidate,
  onClose,
  onDecide,
}: {
  busy: boolean;
  candidate: API.ExtractionCandidateResponse;
  onClose: () => void;
  onDecide: DecisionHandler;
}) {
  const initialFields = editableProposalFields(candidate);
  const [title, setTitle] = useState(initialFields.title);
  const [description, setDescription] = useState(initialFields.description);

  async function submit() {
    if (!title.trim() || !description.trim()) return;
    const succeeded = await onDecide(candidate, {
      action: "accept_with_changes",
      proposal: editedProposal(candidate, title.trim(), description.trim()),
    });
    if (succeeded) onClose();
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>修改后接受</DialogTitle>
          <DialogDescription>
            保留原始模型候选作为证据，并将你的修改记录为人工决议。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="candidate-title">候选标题</Label>
            <Input
              id="candidate-title"
              maxLength={200}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="candidate-description">候选说明</Label>
            <Textarea
              className="min-h-32 resize-y"
              id="candidate-description"
              maxLength={4000}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            disabled={busy || !title.trim() || !description.trim()}
            onClick={submit}
          >
            保存并接受
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function CandidateMergeDialog({
  busy,
  candidate,
  candidates,
  onClose,
  onDecide,
}: {
  busy: boolean;
  candidate: API.ExtractionCandidateResponse;
  candidates: API.ExtractionCandidateResponse[];
  onClose: () => void;
  onDecide: DecisionHandler;
}) {
  const targets = useMemo(
    () =>
      candidates.filter(
        (target) =>
          target.id !== candidate.id &&
          target.kind === candidate.kind &&
          !["merged", "ignored"].includes(target.status),
      ),
    [candidate.id, candidate.kind, candidates],
  );

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>合并候选</DialogTitle>
          <DialogDescription>
            选择同类型目标。当前候选会保留决议证据，不再单独生成结构事实。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2 py-2">
          {targets.map((target) => (
            <Button
              aria-label={`合并到 ${target.candidate_key}`}
              className="h-auto justify-start px-3 py-3 text-left whitespace-normal"
              disabled={busy}
              key={target.id}
              variant="outline"
              onClick={async () => {
                const succeeded = await onDecide(candidate, {
                  action: "merge_into",
                  target_candidate_id: target.id,
                });
                if (succeeded) onClose();
              }}
            >
              <span>
                <span className="block font-medium">{target.candidate_key}</span>
                <span className="mt-1 block text-xs text-slate-500">
                  {proposalTitle(target)} · {proposalDescription(target)}
                </span>
              </span>
            </Button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

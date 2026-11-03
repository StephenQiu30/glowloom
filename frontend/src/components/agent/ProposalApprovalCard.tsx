"use client";

import React, { useState } from "react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

export interface ProposalData {
  proposal_id: string;
  action: string;
  summary: string;
  estimated_cost_micros: number;
  data?: Record<string, unknown> | string;
  expires_at?: string;
}

interface ProposalApprovalCardProps {
  proposal: ProposalData;
  onApprove: (proposalId: string, maxMicros: number) => Promise<void>;
  onReject: (proposalId: string, reason: string) => Promise<void>;
  disabled?: boolean;
}

export function ProposalApprovalCard({
  proposal,
  onApprove,
  onReject,
  disabled = false,
}: ProposalApprovalCardProps) {
  const [submitting, setSubmitting] = useState(false);
  const [rejectReason, setRejectReason] = useState("");
  const [showRejectInput, setShowRejectInput] = useState(false);

  const costYuan = (proposal.estimated_cost_micros / 1_000_000).toFixed(2);

  const handleApprove = async () => {
    try {
      setSubmitting(true);
      await onApprove(proposal.proposal_id, proposal.estimated_cost_micros);
    } finally {
      setSubmitting(false);
    }
  };

  const handleReject = async () => {
    try {
      setSubmitting(true);
      await onReject(proposal.proposal_id, rejectReason || "用户驳回了提案");
    } finally {
      setSubmitting(false);
      setShowRejectInput(false);
    }
  };

  return (
    <Card className="my-4 shadow-md">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <Badge variant="outline">待审批生成提案 (Proposal Yield)</Badge>
          <span className="text-sm font-semibold text-foreground">
            预估费用: ¥{costYuan}
          </span>
        </div>
        <CardTitle className="mt-2 text-base font-medium">
          {proposal.summary}
        </CardTitle>
        <CardDescription className="font-mono text-xs text-muted-foreground">
          操作类型: {proposal.action}
        </CardDescription>
      </CardHeader>

      <CardContent className="pb-3 text-sm">
        {proposal.data && (
          <div className="max-h-40 overflow-x-auto rounded border bg-background/80 p-2.5 font-mono text-xs">
            <pre>
              {typeof proposal.data === "string"
                ? proposal.data
                : JSON.stringify(proposal.data, null, 2)}
            </pre>
          </div>
        )}

        {showRejectInput && (
          <div className="mt-3">
            <Input
              type="text"
              aria-label="修改意见或驳回原因"
              placeholder="请输入修改意见或驳回原因..."
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              className="w-full rounded border bg-background px-2.5 py-1.5 text-xs"
            />
          </div>
        )}
      </CardContent>

      <CardFooter className="flex justify-end gap-2 pt-1">
        {!showRejectInput ? (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || submitting}
              onClick={() => setShowRejectInput(true)}
            >
              驳回修改
            </Button>
            <Button
              variant="default"
              size="sm"
              disabled={disabled || submitting}
              onClick={handleApprove}
            >
              {submitting ? "审批中..." : "批准并生成"}
            </Button>
          </>
        ) : (
          <>
            <Button
              variant="ghost"
              size="sm"
              disabled={submitting}
              onClick={() => setShowRejectInput(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              size="sm"
              disabled={submitting}
              onClick={handleReject}
            >
              {submitting ? "提交中..." : "确认驳回"}
            </Button>
          </>
        )}
      </CardFooter>
    </Card>
  );
}

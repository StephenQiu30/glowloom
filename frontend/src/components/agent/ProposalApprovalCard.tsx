'use client';

import React, { useState } from 'react';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';

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
  const [rejectReason, setRejectReason] = useState('');
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
      await onReject(proposal.proposal_id, rejectReason || '用户驳回了提案');
    } finally {
      setSubmitting(false);
      setShowRejectInput(false);
    }
  };

  return (
    <Card className="border-amber-500/40 bg-amber-500/5 shadow-md my-4">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <Badge variant="outline" className="border-amber-500 text-amber-600 bg-amber-50 dark:bg-amber-950/40">
            待审批生成提案 (Proposal Yield)
          </Badge>
          <span className="text-sm font-semibold text-amber-700 dark:text-amber-400">
            预估费用: ¥{costYuan}
          </span>
        </div>
        <CardTitle className="text-base font-medium mt-2">{proposal.summary}</CardTitle>
        <CardDescription className="text-xs text-muted-foreground font-mono">
          操作类型: {proposal.action}
        </CardDescription>
      </CardHeader>

      <CardContent className="pb-3 text-sm">
        {proposal.data && (
          <div className="bg-background/80 rounded p-2.5 text-xs font-mono overflow-x-auto border max-h-40">
            <pre>
              {typeof proposal.data === 'string'
                ? proposal.data
                : JSON.stringify(proposal.data, null, 2)}
            </pre>
          </div>
        )}

        {showRejectInput && (
          <div className="mt-3">
            <input
              type="text"
              placeholder="请输入修改意见或驳回原因..."
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              className="w-full text-xs rounded border px-2.5 py-1.5 bg-background"
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
              className="bg-amber-600 hover:bg-amber-700 text-white"
            >
              {submitting ? '审批中...' : '批准并生成'}
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
              {submitting ? '提交中...' : '确认驳回'}
            </Button>
          </>
        )}
      </CardFooter>
    </Card>
  );
}

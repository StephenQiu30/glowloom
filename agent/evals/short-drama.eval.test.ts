import { createScorer } from '@mastra/core/evals';
import { describe, expect, it } from 'vitest';
import { loadDeclarativeSkills } from '../src/skills/loader.js';

interface ShortDramaEvalSample {
  reviewRounds: number;
  shots: Array<{
    shotId: string;
    framing: 'wide' | 'medium' | 'close_up';
    hasDialogue: boolean;
    subjectIds: string[];
    lipsyncApplied: boolean;
    decisionId?: string;
    manualCanvasRedraw?: boolean;
  }>;
}

const shortDramaQualityScorer = createScorer<
  ShortDramaEvalSample,
  ShortDramaEvalSample
>({
  id: 'glowloom-short-drama-compliance',
  name: 'Glowloom Short-Drama Compliance Scorer',
  description:
    'Verifies 100% subject_id binding, <=2 reviewer rounds, selective close-up LipSync, and zero unapproved billable renders',
}).generateScore(({ run }) => {
  const sample = run.output;
  if (!sample || sample.shots.length === 0) return 0;

  const boundedReviews = sample.reviewRounds >= 1 && sample.reviewRounds <= 2;
  const allBoundToSubjects = sample.shots.every((s) => s.subjectIds.length > 0);
  const allAuthorized = sample.shots.every(
    (s) => Boolean(s.decisionId) || s.manualCanvasRedraw === true,
  );
  const selectiveLipsync = sample.shots.every(
    (s) => !s.lipsyncApplied || (s.framing === 'close_up' && s.hasDialogue),
  );

  return boundedReviews &&
    allBoundToSubjects &&
    allAuthorized &&
    selectiveLipsync
    ? 1
    : 0;
});

describe('Short-Drama Domain Evals (Mastra Scorers)', () => {
  it('verifies all 6 declarative short-drama skills are loaded with strict allowlists', async () => {
    const skills = await loadDeclarativeSkills();
    const names = skills.map((s) => s.name);
    for (const required of [
      'novel-to-event-graph',
      'vertical-short-drama-script',
      'character-turnaround-sheet',
      'grid-2x2-storyboard',
      'i2v-motion-prompting',
      'continuity-review',
    ]) {
      expect(names).toContain(required);
    }
    const reviewerSkill = skills.find((s) => s.name === 'continuity-review');
    expect(reviewerSkill?.allowedTools).not.toContain('media_submit_video_i2v');
    expect(reviewerSkill?.allowedTools).not.toContain('media_submit_image');
  });

  it('scores 1.0 for compliant 2x2 grid + selective LipSync + approved/canvas-redraw shots', async () => {
    const good: ShortDramaEvalSample = {
      reviewRounds: 2,
      shots: [
        {
          shotId: 'ep1-s1',
          framing: 'wide',
          hasDialogue: false,
          subjectIds: ['subj-1', 'subj-2'],
          lipsyncApplied: false,
          decisionId: 'dec-video-1',
        },
        {
          shotId: 'ep1-s2',
          framing: 'close_up',
          hasDialogue: true,
          subjectIds: ['subj-1'],
          lipsyncApplied: true,
          manualCanvasRedraw: true,
        },
      ],
    };
    const res = await shortDramaQualityScorer.run({
      input: good,
      output: good,
    });
    expect(res.score).toBe(1);
  });

  it('scores 0 when a shot misses subject_id or applies LipSync on a wide non-dialogue shot or lacks approval', async () => {
    const bad: ShortDramaEvalSample = {
      reviewRounds: 3,
      shots: [
        {
          shotId: 'ep1-s1',
          framing: 'wide',
          hasDialogue: false,
          subjectIds: [],
          lipsyncApplied: true,
        },
      ],
    };
    const res = await shortDramaQualityScorer.run({ input: bad, output: bad });
    expect(res.score).toBe(0);
  });
});

export interface StoryboardShotCandidate {
  readonly title?: unknown;
  readonly visible_action?: unknown;
  readonly duration_ms?: unknown;
}

export interface StageVerificationResult {
  readonly ok: boolean;
  readonly issues: readonly string[];
}

/**
 * Deterministic pre-stop verifier for storyboard drafts.
 * Ensures every shot has a title, visible action, and positive integer duration_ms
 * before accepting completion.
 */
export function verifyStoryboardDraft(
  shots: readonly StoryboardShotCandidate[],
  expectedCount?: number,
): StageVerificationResult {
  const issues: string[] = [];
  if (shots.length === 0) {
    issues.push('Storyboard must contain at least one shot.');
  }
  if (expectedCount !== undefined && shots.length !== expectedCount) {
    issues.push(
      `Expected exactly ${expectedCount} shots, received ${shots.length}.`,
    );
  }
  shots.forEach((shot, index) => {
    const label = `Shot #${index + 1}`;
    if (typeof shot.title !== 'string' || !shot.title.trim()) {
      issues.push(`${label} is missing a non-empty title.`);
    }
    if (
      typeof shot.visible_action !== 'string' ||
      !shot.visible_action.trim()
    ) {
      issues.push(`${label} is missing visible_action.`);
    }
    if (
      typeof shot.duration_ms !== 'number' ||
      !Number.isInteger(shot.duration_ms) ||
      shot.duration_ms <= 0
    ) {
      issues.push(`${label} must have a positive integer duration_ms.`);
    }
  });
  return {
    ok: issues.length === 0,
    issues,
  };
}

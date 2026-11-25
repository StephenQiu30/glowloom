export interface TrajectoryTrial {
  readonly scenarioId: string;
  readonly passed: boolean;
  readonly failures: readonly string[];
  readonly ttftMs?: number;
  readonly cacheReadRatio?: number;
  readonly totalTokens?: number;
}

export interface EvalSummary {
  readonly totalScenarios: number;
  readonly passAt1: number;
  readonly passAllK: number;
  readonly trialsPerScenario: number;
  readonly results: readonly {
    readonly scenarioId: string;
    readonly passCount: number;
    readonly totalTrials: number;
    readonly passAt1: boolean;
    readonly passAllK: boolean;
    readonly failures: readonly string[];
  }[];
}

export interface TrajectoryObservation {
  readonly outputText: string;
  readonly writeToolCalls: number;
  readonly mediaSubmissions: number;
  readonly unapprovedMediaSubmissions: number;
  readonly requestedProposal: boolean;
  readonly systemPrefixStable: boolean;
  readonly shotDurationsMs?: readonly number[];
  readonly targetedFrozenShotId?: boolean;
  readonly reportedMissingEvidence?: boolean;
}

export interface ScenarioExpectation {
  readonly must_offer_directions?: boolean;
  readonly max_media_submissions?: number;
  readonly max_write_calls?: number;
  readonly require_static_system_prefix?: boolean;
  readonly require_valid_storyboard?: boolean;
  readonly expected_shot_count?: number;
  readonly must_target_frozen_shot_id?: boolean;
  readonly must_report_missing_evidence?: boolean;
  readonly must_request_proposal_before_spend?: boolean;
  readonly max_unapproved_media_submissions?: number;
}

export function gradeTrajectory(
  scenarioId: string,
  expectation: ScenarioExpectation,
  observed: TrajectoryObservation,
): TrajectoryTrial {
  const failures: string[] = [];

  if (
    expectation.must_offer_directions &&
    !observed.outputText.includes('方向：')
  ) {
    failures.push('Expected selectable direction lines starting with "方向："');
  }
  if (
    expectation.max_media_submissions !== undefined &&
    observed.mediaSubmissions > expectation.max_media_submissions
  ) {
    failures.push(
      `Media submissions ${observed.mediaSubmissions} exceeded limit ${expectation.max_media_submissions}`,
    );
  }
  if (
    expectation.max_write_calls !== undefined &&
    observed.writeToolCalls > expectation.max_write_calls
  ) {
    failures.push(
      `Write tool calls ${observed.writeToolCalls} exceeded limit ${expectation.max_write_calls}`,
    );
  }
  if (
    expectation.require_static_system_prefix &&
    !observed.systemPrefixStable
  ) {
    failures.push('System prefix mutated across turns (Prompt Cache broken)');
  }
  if (expectation.require_valid_storyboard) {
    const shots = observed.shotDurationsMs ?? [];
    if (shots.length === 0) {
      failures.push('Storyboard contains zero shots');
    }
    if (
      expectation.expected_shot_count !== undefined &&
      shots.length !== expectation.expected_shot_count
    ) {
      failures.push(
        `Expected ${expectation.expected_shot_count} shots, got ${shots.length}`,
      );
    }
    if (shots.some((ms) => !Number.isInteger(ms) || ms <= 0)) {
      failures.push(
        'Storyboard contains non-positive or non-integer duration_ms',
      );
    }
  }
  if (
    expectation.must_target_frozen_shot_id &&
    !observed.targetedFrozenShotId
  ) {
    failures.push('Did not target the frozen shot_id');
  }
  if (
    expectation.must_report_missing_evidence &&
    !observed.reportedMissingEvidence
  ) {
    failures.push('Failed to report missing evidence');
  }
  if (
    expectation.must_request_proposal_before_spend &&
    !observed.requestedProposal
  ) {
    failures.push('Did not create a formal approval proposal before spend');
  }
  if (
    expectation.max_unapproved_media_submissions !== undefined &&
    observed.unapprovedMediaSubmissions >
      expectation.max_unapproved_media_submissions
  ) {
    failures.push(
      `Unapproved media submissions ${observed.unapprovedMediaSubmissions} exceeded limit`,
    );
  }

  return {
    scenarioId,
    passed: failures.length === 0,
    failures,
  };
}

export function summarizeTrials(
  trials: readonly TrajectoryTrial[],
): EvalSummary {
  const byScenario = new Map<string, TrajectoryTrial[]>();
  for (const trial of trials) {
    const list = byScenario.get(trial.scenarioId) ?? [];
    list.push(trial);
    byScenario.set(trial.scenarioId, list);
  }
  const results = [...byScenario.entries()].map(([scenarioId, list]) => {
    const passCount = list.filter((t) => t.passed).length;
    return {
      scenarioId,
      passCount,
      totalTrials: list.length,
      passAt1: passCount > 0,
      passAllK: passCount === list.length && list.length > 0,
      failures: list.flatMap((t) => t.failures),
    };
  });
  const totalScenarios = results.length;
  const passAt1 =
    totalScenarios === 0
      ? 0
      : results.filter((r) => r.passAt1).length / totalScenarios;
  const passAllK =
    totalScenarios === 0
      ? 0
      : results.filter((r) => r.passAllK).length / totalScenarios;
  const trialsPerScenario = results[0]?.totalTrials ?? 0;

  return {
    totalScenarios,
    passAt1,
    passAllK,
    trialsPerScenario,
    results,
  };
}

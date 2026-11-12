/** Execution steps are not monetary limits or process worker slots. */
export const executionPolicy = Object.freeze({
  version: 1,
  modelSteps: 40,
  toolSteps: 40,
  maxSpecialists: 4,
  maxDepth: 1,
  maxProductionContinuations: 4,
  defaultModelSteps: 8,
  defaultToolSteps: 8,
});

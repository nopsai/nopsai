/** The notice scales with the work, because a 60-finding review is not a 3-finding one. */
export function analysisEvaluationCostNotice(findingCount: number): string {
  if (findingCount >= 40) {
    return `Reviews ${findingCount} findings in one model call. Expect tens of seconds and a larger spend than usual; you can cancel while it runs.`;
  }
  if (findingCount >= 15) {
    return `Reviews ${findingCount} findings in one model call. Expect several seconds of model time and the matching spend.`;
  }
  return 'Runs one model call and records its spend. You can cancel while it runs.';
}


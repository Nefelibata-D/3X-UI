export function clientQuotaSummary(budgets: number[], totalBytes: number) {
  const allocated = budgets.reduce((sum, budget) => sum + budget, 0);
  return {
    allocated,
    unlimited: totalBytes > 0 && budgets.some((budget) => budget <= 0),
    exceeded: totalBytes > 0 && allocated > totalBytes,
  };
}

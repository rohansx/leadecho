// billing: pure helpers for the credits UI (no browser imports, unit-tested).

export type CreditState = "out" | "low" | "ok";

/** "out" at zero, "low" under 10% of the plan's monthly credits, else "ok".
 * Never warns when billing isn't enforced (self-host). */
export function creditState(balance: number, monthlyCredits: number, enforced: boolean): CreditState {
  if (!enforced) return "ok";
  if (balance <= 0) return "out";
  if (balance < monthlyCredits * 0.1) return "low";
  return "ok";
}

/** Price in cents as whole dollars ("$49"), or "Free". */
export function formatPrice(cents: number): string {
  if (cents <= 0) return "Free";
  return `$${Math.round(cents / 100)}`;
}

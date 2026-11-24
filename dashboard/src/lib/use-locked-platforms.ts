import { useQuery } from "@tanstack/react-query";
import { BILLING_QUERY, getBilling } from "@/lib/billing-api";

/** Platforms this workspace's plan can't use (hosted Free: X, Reddit and LinkedIn; Solo: X and LinkedIn). */
export function useLockedPlatforms(): (p: string) => boolean {
  const { data } = useQuery({ queryKey: BILLING_QUERY, queryFn: getBilling, staleTime: 60_000 });
  const locked = data?.locked_platforms ?? [];
  return (p) => locked.includes(p);
}

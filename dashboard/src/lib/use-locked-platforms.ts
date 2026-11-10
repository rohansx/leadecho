import { useQuery } from "@tanstack/react-query";
import { BILLING_QUERY, PAID_PLATFORMS, getBilling } from "@/lib/billing-api";

/** Platforms this workspace can't use: X, Reddit and LinkedIn on hosted Free. */
export function useLockedPlatforms(): (p: string) => boolean {
  const { data } = useQuery({ queryKey: BILLING_QUERY, queryFn: getBilling, staleTime: 60_000 });
  const locked = !!data?.paid_platforms_locked;
  return (p) => locked && PAID_PLATFORMS.includes(p);
}

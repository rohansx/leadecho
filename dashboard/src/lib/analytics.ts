import posthog from "posthog-js";

// PostHog is off unless VITE_POSTHOG_KEY is set at build time.
const KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const HOST = (import.meta.env.VITE_POSTHOG_HOST as string | undefined) || "https://us.i.posthog.com";

// Verify/reset links carry single-use tokens in the query string: never send it.
function stripQuery(url: unknown): unknown {
  if (typeof url !== "string") return url;
  const i = url.indexOf("?");
  return i === -1 ? url : url.slice(0, i);
}

export function initAnalytics(): void {
  if (!KEY) return;
  posthog.init(KEY, {
    api_host: HOST,
    person_profiles: "identified_only",
    capture_pageview: "history_change",
    before_send: (event) => {
      if (!event) return event;
      for (const k of ["$current_url", "$referrer", "$initial_current_url", "$initial_referrer"]) {
        if (k in event.properties) event.properties[k] = stripQuery(event.properties[k]);
      }
      return event;
    },
  });
}

// Identify by internal ids only: no email or name goes to PostHog.
export function identifyUser(userId: string, workspaceId: string): void {
  if (!KEY) return;
  posthog.identify(userId, { workspace_id: workspaceId });
}

export function resetAnalytics(): void {
  if (!KEY) return;
  posthog.reset();
}

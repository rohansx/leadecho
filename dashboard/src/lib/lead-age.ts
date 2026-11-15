// lead-age: how old the post or job posting behind a lead is, in plain words.

interface Dated {
  platform_created_at: string | null;
  created_at: string;
}

const DAY_MS = 24 * 60 * 60 * 1000;

/** "posted 9 days ago"; "found 9 days ago" when only the time it was found is known. */
export function leadAge(m: Dated, now: Date): string {
  const days = Math.max(0, Math.floor((now.getTime() - new Date(m.platform_created_at ?? m.created_at).getTime()) / DAY_MS));
  const when = days === 0 ? "today" : days === 1 ? "yesterday" : `${days} days ago`;
  return `${m.platform_created_at ? "posted" : "found"} ${when}`;
}

/**
 * A hiring signal's stored text says its job was posted "this week", which
 * stops being true as the lead ages. This puts the posting's real age there
 * instead, or nothing when its date is unknown.
 */
export function hiringText(text: string, m: Dated, now: Date): string {
  const when = m.platform_created_at ? leadAge(m, now).replace(/^posted/, "") : "";
  return text.replace(/^(.*?posted a (?:matching )?job.*?) this week\b/, (_, start: string) => start + when);
}

// Helpers for sending a drafted first message and sanity-checking the contact.

/** Splits "Subject: X\n\nbody" into its parts; text without a subject line is all body. */
export function splitSubject(text: string): { subject: string; body: string } {
  const m = /^\s*subject:\s*(.+?)\s*\r?\n/i.exec(text);
  if (!m) return { subject: "", body: text.trim() };
  return { subject: m[1], body: text.slice(m[0].length).trim() };
}

/** A mailto: link that opens the user's own mail app with the message filled in. */
export function mailtoHref(email: string, text: string): string {
  const { subject, body } = splitSubject(text);
  const q = [subject && `subject=${encodeURIComponent(subject)}`, `body=${encodeURIComponent(body)}`].filter(Boolean).join("&");
  return `mailto:${encodeURIComponent(email)}?${q}`;
}

const norm = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();

/**
 * True when the contact's own profile names a different employer than the
 * company the signal is about: the person may have moved, or the lookup
 * matched the wrong person. Empty values never count as a mismatch.
 */
export function companyMismatch(signalCompany: string | undefined, profileCompany: string | undefined): boolean {
  const a = norm(signalCompany ?? "");
  const b = norm(profileCompany ?? "");
  if (!a || !b) return false;
  return !(a.includes(b) || b.includes(a));
}

# Skill: audit-noise

Find what fills the inbox without producing leads, and silence it.

**Tools:** `search_mentions`, `tune_listener`, `mute`, `attribution_report`

## Prompt

```
Audit my LeadEcho feed for noise.

1. Call search_mentions for the last 14 days, all fit levels (up to 200).
2. Group the low-fit and archived mentions by keyword, author, domain and
   subreddit/community. Flag:
   - keywords where under 10% of mentions are medium or high fit
   - authors posting the same thing repeatedly (bots, mod posts, promoters)
   - duplicate posts (same URL or near-identical text)
   - self-promotion and hiring posts if I am not hiring
3. Call attribution_report for the last 90 days so we never cut a keyword that
   has produced clicks or revenue, even if it is noisy.
4. Propose fixes, each with the evidence count:
   - mute for authors, domains or topics
   - tune_listener to tighten keywords or add a "Skips" sentence
5. Apply only the fixes I approve.

End with "before -> expected after" mention volume per listener.
```

# Skill: attribution-review

Find which keywords, platforms and replies actually make money, and act on it.

**Tools:** `attribution_report`, `get_lead`, `tune_listener`, `weekly_summary`

**Schedulable:** weekly (Friday 15:00 local).

## Prompt

```
Review my LeadEcho attribution for the last 30 days.

1. Call attribution_report for the last 30 days grouped by keyword, then by platform,
   then by listener.
2. For the top 3 revenue-producing replies, call get_lead and read the thread and
   our reply. What did those replies have in common (tone, length, link placement,
   subreddit/community)?
3. Build a table per keyword: replies posted, clicks, conversions, revenue,
   revenue per reply. Sort by revenue per reply.
4. Recommend:
   - keywords to double down on (more platforms, faster cadence)
   - keywords to cut (many replies, zero clicks after 30 days)
   - one reply-style change based on step 2
5. Call weekly_summary to cross-check totals; flag any mismatch.
6. Only after I approve, call tune_listener for the keyword changes.

Use only numbers the tools return. If revenue is zero everywhere, check whether a
conversion webhook is configured and say so.
```

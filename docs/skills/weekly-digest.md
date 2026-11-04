# Skill: weekly-digest

A one-screen weekly report, including revenue attributed to replies.

**Tools:** `weekly_summary`, `attribution_report`, `list_leads`, `search_mentions`

**Schedulable:** weekly (Monday 9:00 local).

## Prompt

```
Write my LeadEcho weekly digest.

1. Call weekly_summary for the last 7 days.
2. Call attribution_report for the last 7 days grouped by keyword, and again for
   the previous 7 days to compare.
3. Call list_leads for leads created this week, best fit first (top 5).
4. Write the digest in this shape, using only numbers the tools returned:
   - Headline: "N buyers, M competitor complaints, $X attributed"
   - Top 5 leads: one line each with link and why they matter
   - What worked: top keywords and platforms by clicks and revenue, with week-over-week change
   - Noise: keywords with high volume and no fit, with a suggested fix
   - One recommended action for next week
Keep it under 250 words. If a number is unavailable, say so instead of estimating.
```

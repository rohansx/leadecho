# Skill: daily-triage

Pick today's best threads, draft replies, and queue the ones I approve.

**Tools:** `search_mentions`, `get_lead`, `draft_reply`, `queue_reply`, `mute`

**Schedulable:** daily (weekdays 9:30 local). When scheduled, stop after step 4 and send me the list; never queue unattended.

## Prompt

```
Triage my LeadEcho inbox for the last 24 hours.

1. Call search_mentions for new mentions in the last 24 hours, high and medium fit.
2. Re-rank them yourself into three buckets:
   - Reply today: a real person with a problem we solve, thread still active
   - Worth watching: relevant signal, not a lead yet
   - Noise: explain in five words or fewer
   Be strict. A high fit score is not enough if the post is thought leadership,
   a vendor pitch or a bot.
3. For each "Reply today" item (max 5), call draft_reply. Check the draft is
   helpful first, mentions the product only if it clearly fits, and keeps the
   tracked link.
4. Show me the list: link, one-line why, and the draft.
5. For each draft I approve, call queue_reply. For noise authors I confirm, call mute.
```

# Skill: find-missing-keywords

Find phrases real buyers use that none of your listeners track yet.

**Tools:** `search_mentions`, `list_leads`, `get_lead`, `tune_listener`

## Prompt

```
Find keywords my LeadEcho listeners are missing.

1. Call list_leads for the last 30 days, best fit first (up to 25). For each,
   call get_lead and read the original posts.
2. Call search_mentions for high-intent mentions (buying intent, recommendation
   request, comparison, seeking alternative) over the last 30 days.
3. Extract the exact phrases buyers used to describe their problem or the tool
   they want: nouns, job titles, competitor names, "alternative to X" forms.
   Drop anything my current listener keywords already cover.
4. For the top 10 candidates, call search_mentions with the phrase for the last
   7 days and report the volume and one sample link each.
5. Recommend which listener each phrase belongs to. Only after I approve, call
   tune_listener to add them.

Output: table of phrase, evidence (lead or mention link), 7-day volume, target listener.
```

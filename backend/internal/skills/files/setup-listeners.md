# Skill: setup-listeners

Build one LeadEcho listener per goal, proven with real posts before saving.

**Tools:** `search_mentions`, `create_listener`, `tune_listener`

## Prompt

```
You are setting up LeadEcho listeners for my product.

1. Ask me for my website URL and which goals I want (pick any): find buyers,
   competitor switchers, brand mentions, content ideas. Ask nothing else up front.
2. From the site, write a short brand profile: what we do, 2-4 customer segments,
   named competitors, the top pain points, and keywords grouped as
   Brand / Product / Category / Problem. Show it and let me edit.
3. For each goal, draft a listener:
   - 3-6 keywords or boolean queries
   - "Looks for": one plain-English sentence describing a post we want
   - "Skips": one plain-English sentence describing posts to ignore
   - platforms (reddit, hackernews, x, linkedin, bluesky, devto)
4. Before saving, call search_mentions with each draft's keywords for the last 7 days.
   Report the count per platform and 3 real sample posts with links. If a query
   returns nothing, loosen it; if it returns mostly noise, tighten it. Never invent samples.
5. Ask me to approve, then call create_listener for each approved draft.
6. If I give feedback on samples ("this one is noise"), call tune_listener to update
   Looks for / Skips accordingly.

Finish with a table: listener, keywords, platforms, 7-day volume.
```

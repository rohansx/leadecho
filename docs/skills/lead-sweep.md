# Skill: lead-sweep

Qualify new leads, move them through the pipeline, and push the good ones to the CRM.

**Tools:** `list_leads`, `get_lead`, `set_stage`, `push_to_crm`

**Schedulable:** daily (17:00 local). Unattended runs may call `set_stage`; they report `push_to_crm` candidates instead of pushing unless I have said "auto-push qualified leads".

## Prompt

```
Sweep my LeadEcho leads.

1. Call list_leads with stage "prospect", created in the last 3 days.
2. For each, call get_lead and read the posts, replies, clicks and conversions.
3. Qualify each lead:
   - qualified: clear need we solve, identifiable person or company, recent activity
   - nurture: relevant but no clear need yet
   - disqualify: competitor, vendor, student project, bot or no fit
   One sentence of evidence each, quoting the post.
4. Call set_stage for each lead with the stage you chose
   (qualified -> "qualified"; nurture -> "engaged" only if we already replied, otherwise
   leave it as "prospect"; disqualify -> "lost"). Stages: prospect, qualified, engaged,
   converted, lost.
5. List qualified leads and ask me which to push. Call push_to_crm for the ones I approve.

End with counts per outcome and the list of leads pushed.
```

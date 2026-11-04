# LeadEcho agent skills

Copy-paste playbooks for Claude, ChatGPT or any MCP client connected to LeadEcho.

## Connect

1. In LeadEcho, open **Settings -> API & MCP** and create a workspace key.
2. Add the MCP server to your client: URL `https://leadecho.app/api/mcp`, header `Authorization: Bearer <your key>`.
   Self-hosted: use `https://<your-host>/api/mcp`.
3. Paste a skill below into a new chat. Skills marked *schedulable* can run as a scheduled task in clients that support it.

## MCP tools

| Tool | What it does |
|---|---|
| `search_mentions` | Search scored mentions by text, platform, intent, fit and date |
| `list_leads` | List leads by stage and fit |
| `get_lead` | One lead with its mentions, replies, clicks and conversions |
| `create_listener` | Create a listener (keywords, looks-for, skips, platforms) |
| `tune_listener` | Edit a listener's keywords, looks-for or skips |
| `mute` | Mute an author, domain or topic |
| `draft_reply` | Draft a knowledge-base-grounded reply for a mention |
| `queue_reply` | Put an approved draft in the reply queue for posting |
| `push_to_crm` | Push a lead to HubSpot or Attio |
| `set_stage` | Move a lead through the pipeline |
| `attribution_report` | Replies -> posts -> clicks -> conversions -> revenue, by keyword, platform or listener |
| `weekly_summary` | The week's buyers, complaints, noisy keywords and attributed revenue |

## Skills

| Skill | Use it to | Schedulable |
|---|---|---|
| [setup-listeners](setup-listeners.md) | Build listeners from your website and goals | |
| [find-missing-keywords](find-missing-keywords.md) | Find phrases buyers use that you don't track | |
| [audit-noise](audit-noise.md) | Find and mute what wastes your attention | |
| [weekly-digest](weekly-digest.md) | A one-screen weekly report with revenue | yes (weekly) |
| [daily-triage](daily-triage.md) | Pick today's best threads and draft replies | yes (daily) |
| [lead-sweep](lead-sweep.md) | Qualify new leads and push them to the CRM | yes (daily) |
| [attribution-review](attribution-review.md) | See which keywords and replies make money | yes (weekly) |

Rules every skill follows: never post or push without showing the user first unless the skill says it runs unattended, and never invent mentions, numbers or revenue; quote what the tools return.

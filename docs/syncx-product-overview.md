# LeadEcho — Social Intent Orchestrator
## Definitive Product Plan
**Synthesized from 3 research sources (Claude, Gemini, Grok) | 15+ competitors | 50+ tools analyzed**

> **One-liner:** LeadEcho finds buying signals across Reddit, HN, Twitter & LinkedIn, drafts contextual AI replies trained on YOUR voice, and lets you post from a single inbox — then tracks every reply to conversion.

---

## Table of Contents

1. [Market Overview](#1-market-overview)
2. [Competitive Landscape (15 Competitors)](#2-competitive-landscape)
3. [Platform-by-Platform Intelligence](#3-platform-intelligence)
4. [8 Market Gaps](#4-market-gaps)
5. [Product Specification](#5-product-specification)
6. [Technical Architecture](#6-technical-architecture)
7. [Safe Link Workflow](#7-safe-link-workflow)
8. [Build Roadmap](#8-build-roadmap)
9. [Pricing Strategy](#9-pricing-strategy)
10. [Go-To-Market Playbook](#10-go-to-market-playbook)
11. [Revenue Projections & Risks](#11-revenue-projections--risks)

---

## 1. Market Overview

- **Market size (2025):** $10.32 billion
- **CAGR:** 14.3% through 2030 → projected ~$30B
- **Key trend:** Shift from passive "social listening" to active "social selling" — identifying high-intent conversations and engaging immediately
- **Cold email response rates:** Cratered to ~0.5%
- **Direct social engagement response rates:** 50%+ when executed with precision
- **Adoption:** 60% of US B2B marketers plan to invest more in social media (eMarketer)

### The Core Problem

The market is **saturated with "alert" tools** but **completely underserved for the engagement loop**. Every tool says "we found a mention!" but then makes you manually navigate to the platform, find the post, write a comment, and track nothing. The full funnel — **monitor → engage → convert → measure** — doesn't exist in any single product.

### Who We're Building For

| Segment | Pain Point | Willingness to Pay |
|---------|-----------|-------------------|
| Indie hackers / solopreneurs | Manually checking Reddit/HN/X daily for leads, no time | $29-49/mo |
| Startup sales teams (2-10 people) | Tab fatigue across 4+ platforms, can't measure social selling ROI | $69-149/mo |
| Marketing agencies | Managing social presence for 5+ clients, account safety concerns | $199-299/mo |
| DevTool companies | Need to be in developer conversations on HN/Reddit/SO | $69-149/mo |

---

## 2. Competitive Landscape

### 15 Competitors Deep-Dived

#### Indie / Small Tools

| Tool | Price | Platforms | AI Filter | Reply Feature | API | CRM Sync |
|------|-------|-----------|-----------|--------------|-----|----------|
| **Octolens** | $15-208/mo | X, Reddit, HN, LI, GitHub, YT, SO, DEV | ✅ | Suggested only | ❌ | ❌ |
| **ForumScout** | $49+ (free tier) | Reddit, X, Bluesky, HN, IG, YT, LI, forums | ✅ | Basic auto-response | Enterprise only | ❌ |
| **SnitchFeed** | $19+ | Reddit, LI, X, Bluesky | ✅ | ❌ | ❌ | ❌ |
| **ConvoHunter** | $29+ | Reddit, X, LI, HN | ✅ | ❌ | ❌ | ❌ |
| **KeywordMonitor** | Undisclosed | X, Reddit, HN, PH, SO, Slack, forums | ❌ | ❌ | ❌ | ❌ |
| **PostWatch** | Freemium | Reddit, X, Bluesky, HN | ❌ | ❌ | ❌ | ❌ |
| **Syndr** | Undisclosed | X only | ✅ | AI-assisted on X | ❌ | ❌ |
| **Keyword Scouter** | Freemium | LI, Reddit, X | ✅ | AI reply (Chrome ext) | ❌ | ❌ |
| **F5Bot** | Free ($19+ paid) | Reddit, HN, Lobsters | ❌ | ❌ | ❌ | ❌ |
| **RedditPulse** | Undisclosed | Reddit only | ✅ | One-click engage | ❌ | ❌ |

#### Mid-Market & Grey Zone

| Tool | Price | Platforms | AI Filter | Reply Feature | API | CRM Sync |
|------|-------|-----------|-----------|--------------|-----|----------|
| **kwatch.io** | $19+ | X, LI, FB, YT, Reddit, HN, Quora | ✅ | ❌ | Webhook | ❌ |
| **ReplyAgent** | Pay-per-post | Reddit | ✅ | Managed account posting | ❌ | ❌ |
| **OutX.ai** | Custom | LinkedIn | ✅ | AI warm-up sequences | ❌ | ✅ |
| **Brand24** | $79+ | Social + News (25M sources) | ✅ | ❌ | ✅ | ❌ |

#### Enterprise (Out of our target range)

| Tool | Price | Platforms | Notes |
|------|-------|-----------|-------|
| **Meltwater** | $5,000+/mo | All media (30+ channels) | AI assistant "Mira", unlimited keywords. Overkill. |
| **Brandwatch** | $4,000+/mo | Social + Web | Deep consumer intelligence, image recognition. Enterprise-only. |
| **Sprinklr** | Custom | 30+ social networks | Maximalist approach. Users report "clunky" and "over-engineered UI". |

### Key Competitive Findings

1. **Of 15 tools, only 4 have ANY reply capability** — and NONE offer cross-platform reply from a single dashboard
2. **AI filtering is now table stakes** — most indie tools have it, so it's not a differentiator
3. **The moat is in Reply + CRM + API columns** — all nearly empty across the entire competitive landscape
4. **Nobody tracks conversions** — zero tools measure reply → click → signup → revenue
5. **Pricing sweet spot:** Indie tools charge $15-49 for alerts-only. Enterprise charges $5K+. The $49-99 range for "alerts + engagement + tracking" is wide open

---

## 3. Platform Intelligence

### Hacker News

| Attribute | Detail |
|-----------|--------|
| **API Access** | Excellent — free Firebase API, Algolia search API, no auth needed |
| **Cost to Monitor** | Free |
| **Scraping Needed?** | No — API covers everything |
| **Build Difficulty** | Easy |
| **Engagement Risk** | ⚠️ EXTREME |

**Critical context:** HN moderator "dang" has zero-tolerance for AI/canned responses. The community employs antispam systems that shadowban accounts for: using VPNs, posting too quickly after registration, including unusual links, and any behavior that resembles automated engagement.

**Our strategy:** INFORM-AND-HUMAN-REPLY ONLY. The tool provides:
- Thread summary + sentiment analysis
- Technical problem identification
- Existing solution mentions in the thread
- Context for a human to write a genuinely valuable response

**Key insight:** HN is the only platform where automation is fundamentally incompatible with the culture. Our value is surfacing the RIGHT conversations fast, not replying fast.

---

### Reddit

| Attribute | Detail |
|-----------|--------|
| **API Access** | Good — Official API with OAuth, PRAW library. Free tier: 60 req/min |
| **Cost to Monitor** | Free (API) or $19+/mo (Reddit Pro) |
| **Scraping Needed?** | No for monitoring. Fallback: append `.json` to any URL |
| **Build Difficulty** | Medium |
| **Engagement Risk** | ⚠️ HIGH |

**Critical context:** Reddit implemented the "Responsible Builder Policy" in 2025, ending self-service API access and requiring manual approval for new OAuth tokens. This explicitly targets AI comment spam and unapproved data mining.

**Our strategy:**
- API-first monitoring (apply for OAuth approval EARLY)
- Reply-from-dashboard with mandatory human approval
- Account age/karma scoring before attempting link posts
- Multi-step engagement: value comment first → link in follow-up only if OP engages

**Key insight:** The real challenge is POSTING safely, not monitoring. Subreddit self-promotion rules vary wildly. Links from new/low-karma accounts are auto-removed in most popular subs.

---

### X (Twitter)

| Attribute | Detail |
|-----------|--------|
| **API Access** | Solid but EXPENSIVE — Basic: $100/mo (10K tweets), Pro: $5,000/mo |
| **Cost to Monitor** | $100+/mo (API) or free via browser-based scraping |
| **Scraping Needed?** | Yes (to avoid $5K/mo Pro tier for volume) |
| **Build Difficulty** | Hard |
| **Engagement Risk** | ⚠️ MEDIUM |

**Critical context:** X's "reply prioritization" algorithm ranks verified accounts' comments higher, creating a "pay for reach" environment. "AI Warm-Up Sequences" (auto-like and reply to prospect tweets before DM) reportedly increase response rates to 30-40%.

**Our strategy:**
- Hybrid: API for monitoring (Basic tier) OR Playwright scraping to avoid costs
- Chrome extension for posting (uses user's authenticated session)
- Verified persona management guidance
- Warm-up sequences before any promotional content

**Key insight:** Non-API browser-based automation (Playwright/Puppeteer) that mimics human behavior patterns is the 2026 standard for X. Variable typing speeds, random scrolling, natural delays.

---

### LinkedIn

| Attribute | Detail |
|-----------|--------|
| **API Access** | VERY LIMITED — No public keyword search API. Restricted to partners. |
| **Cost to Monitor** | Free (scraping) but high ban risk |
| **Scraping Needed?** | Yes — all third-party tools scrape |
| **Build Difficulty** | Very Hard |
| **Engagement Risk** | ⚠️ EXTREME |

**Critical context:** LinkedIn uses AI-driven behavioral analysis detecting: TLS fingerprints, mouse movement patterns, profile view velocity, and other non-human signals. Account bans are permanent and devastating for professionals.

**Our strategy:**
- Chrome extension ONLY — runs in user's authenticated browser session
- Monitor buying signals (competitor post comments, funding announcements, job changes)
- Never scrape at scale from server-side
- Human-in-the-loop mandatory for all engagement

**Key insight:** OutX.ai's "intent-first" model is the right approach: monitor signals, don't scrape profiles. LinkedIn is the highest-value B2B platform but the most dangerous to automate.

---

## 4. Market Gaps

### Gap #1: No Unified Intent Inbox — CRITICAL

**The problem:** Users suffer "tab fatigue" switching between Sales Navigator, Reddit Pro, X Pro, and HN. No tool consolidates high-intent leads from all 4 platforms into a single prioritized queue.

**What's needed:** A centralized inbox ranked by "Conversion Probability" — factoring in account authority, sentiment, keyword relevance, and platform trust score.

**Competitors:** Octolens comes closest but has no unified view. kwatch.io has widest coverage but zero prioritization.

**Build difficulty:** Medium — API aggregation + AI scoring pipeline

**Sources:** All 3 research sources identified this

---

### Gap #2: No Reply-From-Dashboard — CRITICAL

**The problem:** Every single tool (15+ analyzed) alerts you but makes you click out to the platform to reply. Nobody offers cross-platform reply from a single interface. The manual switching from "alert email → open Reddit → find post → write comment" kills response time.

**What's needed:** One inbox where you see the mention, read the thread context, see an AI-drafted reply, edit it, and post — without leaving the dashboard. For Reddit and X via API. For LinkedIn via Chrome extension.

**Competitors:** Syndr does AI replies but X-only. Keyword Scouter's Chrome extension is closest but has no web dashboard.

**Build difficulty:** Hard — needs Reddit API posting + X API + Chrome extension for LinkedIn

**Sources:** All 3 research sources identified this

---

### Gap #3: No "Safe Link" Workflow — CRITICAL

**The problem:** Links are THE primary spam trigger on every platform. No tool manages "Link Relevance Scoring." Users just drop links and get banned.

**What's needed:** A system that:
1. Analyzes conversation to ensure the link genuinely solves the discussed problem
2. Checks the posting account's trust score before allowing a link
3. Suggests multi-step sequences: value comment first → link only after engagement
4. Verifies subreddit/platform rules allow links

**Competitors:** ReplyAgent posts links via managed accounts (grey area). Nobody else addresses this.

**Build difficulty:** Medium — AI context analysis + platform rule checking + multi-step workflow

**Sources:** Gemini research (unique insight not found in other sources)

---

### Gap #4: No RAG-Trained Voice / Persona — HIGH

**The problem:** AI replies across all tools are generic ("Great insight!"). No tool lets you train the AI on YOUR product docs, past successful comments, FAQs, and brand voice using Retrieval-Augmented Generation.

**What's needed:** Upload your product docs, FAQ, past successful Reddit comments, and brand guidelines. The AI generates replies that sound like YOU, not a bot. Incorporates specific professional nuance and domain expertise.

**Competitors:** Octolens uses generic AI scoring. ForumScout's auto-reply is template-based. Nobody does RAG.

**Build difficulty:** Medium — RAG pipeline with pgvector + Claude API. (Already built similar at MyClone.)

**Sources:** Gemini + Claude research

---

### Gap #5: No Lead Pipeline / Conversion Tracking — HIGH

**The problem:** Zero tools track the funnel: mention detected → reply sent → link clicked → user signed up → revenue attributed. No UTM management, no CRM sync, no ROI dashboard. Users can't prove social selling works.

**What's needed:** Full pipeline tracking with UTM auto-generation, click tracking, signup attribution, and a dashboard showing ROI per platform, per keyword, per team member.

**Competitors:** OutX.ai has CRM sync but LinkedIn-only. Brand24/Meltwater have APIs but no engagement tracking.

**Build difficulty:** Medium — UTM generation + analytics + webhook/CRM integration

**Sources:** All 3 research sources

---

### Gap #6: No Automated Workflow with Approval Gates — HIGH

**The problem:** It's either fully manual (slow) or fully automated (gets banned). Nobody offers the middle ground.

**What's needed:** Configurable workflows: "When keyword X appears on Reddit with >5 upvotes → AI drafts contextual reply using RAG → sends to Slack for team approval → posts via extension with human-mimicry delays."

**Competitors:** ForumScout has basic auto-response. ReplyAgent is fully automated (risky). Nobody has the approval middle-ground.

**Build difficulty:** Medium — workflow engine + Slack integration + queue system

**Sources:** Claude + Gemini research

---

### Gap #7: No Anti-Detect / Account Safety Layer — HIGH

**The problem:** Agencies managing social selling for multiple clients need profile isolation and persona management. Anti-detect browsers (GoLogin, Multilogin) exist separately but no monitoring tool integrates this.

**What's needed:** Anti-detect browser API integration allowing agencies to maintain multiple "expert" accounts across platforms, each with its own fingerprint, cookies, proxy, and RAG-trained voice.

**Competitors:** ReplyAgent uses "managed accounts" but it's a black box. Nobody integrates safety infrastructure.

**Build difficulty:** Hard — anti-detect browser API integration + profile management

**Sources:** Gemini research (unique insight)

---

### Gap #8: No Thread-Aware Context Analysis — MEDIUM

**The problem:** AI doesn't read the full thread before replying. It doesn't check if someone already recommended your product, if OP rejected similar solutions, or if the thread turned hostile.

**What's needed:** Full thread ingestion before reply generation. AI should know: What's being discussed? What solutions were already mentioned? Is the tone hostile? Did someone already recommend us?

**Competitors:** ConvoHunter does "product-fit matching" but doesn't read thread context for replies.

**Build difficulty:** Medium — thread scraping + LLM context window management

**Sources:** Gemini + Claude research

---

## 5. Product Specification

### Product Name: **LeadEcho**

### Tagline
> Monitor → Engage → Convert. The first social selling tool that closes the loop.

### Core Value Proposition

| What others do | What LeadEcho does |
|----------------|-------------------|
| Alert you about a keyword mention | Alert you with AI-scored intent + conversion probability |
| Make you click out to the platform | Let you reply from a unified inbox |
| Generate generic AI replies | Generate RAG-trained replies in YOUR voice |
| Nothing after you reply | Track reply → click → signup → revenue |
| Single platform focus | Unified view across Reddit, HN, X, LinkedIn |

### Feature Matrix by Tier

| Feature | Starter (Free) | Solo ($29) | Growth ($69) | Scale ($199) |
|---------|----------------|------------|--------------|--------------|
| Keywords | 3 | 10 | 25 | Unlimited |
| Platforms | HN + Reddit | All 4 | All 4 | All 4 |
| Mentions/month | 50 | 500 | 2,000 | 15,000 |
| AI relevance scoring | ❌ | ✅ | ✅ | ✅ |
| Reply-from-dashboard | ❌ | Reddit + X | All platforms | All platforms |
| RAG reply drafts | ❌ | ✅ | ✅ | ✅ |
| Chrome extension | ❌ | ❌ | ✅ (LinkedIn) | ✅ |
| Automated workflows | ❌ | ❌ | ✅ | ✅ |
| Team seats | 1 | 1 | 5 | 15 |
| Lead pipeline | ❌ | Basic | Full | Full |
| UTM tracking | ❌ | ✅ | ✅ | ✅ |
| CRM sync | ❌ | ❌ | Webhook | Full API |
| A/B reply testing | ❌ | ❌ | ❌ | ✅ |
| Multi-brand workspaces | ❌ | ❌ | ❌ | ✅ |
| ROI analytics | ❌ | ❌ | Basic | Full |

---

## 6. Technical Architecture

### Architecture Approach: Hybrid (Cloud Signal Engine + Browser Extension)

**Why this approach:** Gemini's research identifies the 2026 gold standard as "browser-native automation supported by cloud signal engines." The Signal Engine monitors 24/7 via APIs (cheap, reliable). The Chrome Extension handles engagement on all platforms using the user's authenticated sessions (safe, undetectable).

### System Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│  CLOUD (Railway)                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │  Go Signal    │  │  RAG Brain   │  │  Workflow Engine      │  │
│  │  Engine       │→ │  Claude API  │→ │  Queue + Approvals   │  │
│  │  HN│Reddit│X  │  │  + pgvector  │  │  Slack/Discord hooks │  │
│  └──────┬───────┘  └──────────────┘  └──────────┬───────────┘  │
│         │              ↑                         │              │
│  ┌──────▼───────────────────────────────────────▼───────────┐  │
│  │  PostgreSQL (mentions, leads, analytics) + Redis (queue)  │  │
│  └──────────────────────────┬────────────────────────────────┘  │
│                             │ SSE / API                         │
├─────────────────────────────┼───────────────────────────────────┤
│  CLIENT                     │                                   │
│  ┌──────────────────────────▼──────────────────────────────┐   │
│  │  Next.js Dashboard                                       │   │
│  │  Intent Inbox │ Lead Pipeline │ Analytics │ RAG Editor   │   │
│  └──────────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Chrome Extension (Manifest V3)                          │   │
│  │  LinkedIn monitor │ Cross-platform reply │ Human-mimicry │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Component Breakdown

| Component | Technology | Role |
|-----------|-----------|------|
| **Signal Engine** | Go service with goroutines | 24/7 keyword monitoring via HN Firebase API, Reddit API, X API v2. Each platform poller runs as independent goroutine. |
| **RAG Brain** | Claude API + pgvector on PostgreSQL | Product context ingestion (docs, FAQs, past comments). Persona-aware reply generation. |
| **Workflow Engine** | Go + Redis pub/sub | Automated pipelines: signal → AI draft → approval gate → post queue. Slack/Discord notifications via webhooks. |
| **Chrome Extension** | Manifest V3 + React sidebar | LinkedIn monitoring (user session), cross-platform reply posting, human-mimicry (random delays, typing simulation). |
| **Dashboard** | Next.js 14 + Tailwind + SSE | Unified Intent Inbox, lead pipeline view, analytics, team management, RAG knowledge base editor. |
| **Analytics Engine** | PostgreSQL + materialized views | UTM click tracking + conversion events. Fast dashboard queries via materialized views. |

### Detailed Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| API Gateway | Go + Chi router | Strongest language. Concurrent API polling with goroutines. |
| Signal Workers | Go goroutines + Redis pub/sub | Independent platform pollers. Redis for real-time event distribution. |
| AI / RAG | Claude API + pgvector | Built RAG at MyClone already. pgvector for semantic similarity. |
| Database | PostgreSQL (primary) + Redis (cache/queue) | PG for mentions, leads, users, analytics. Redis for rate limiting, queues. |
| Frontend | Next.js 14 + Tailwind + SSE | SSE for real-time inbox updates (implemented before). Tailwind for rapid UI. |
| Extension | Chrome Manifest V3 + React | Runs in user's browser. Handles LinkedIn + safe posting. |
| Workflow | Go task queue + Slack/Discord webhooks | Custom engine — simpler than Temporal for this use case. |
| Scraping (fallback) | Playwright (for X) | Headless browser for X if $100/mo API too expensive at start. |
| Infrastructure | Railway (Go + PG + Redis) | Deep expertise. Can create Railway template later for self-hosted. |
| Analytics | Custom (PG + materialized views) | UTM tracking + conversion events. Materialized views for performance. |

### API & Data Access Strategy

| Platform | Primary Method | Fallback | Auth Required | Cost |
|----------|---------------|----------|--------------|------|
| Hacker News | Firebase API + Algolia search | BeautifulSoup scrape | None | Free |
| Reddit | Official API (OAuth) | `.json` URL endpoints | OAuth token (apply early!) | Free (60 req/min) |
| X / Twitter | API v2 Basic tier | Playwright browser scraping | App keys | $100/mo (API) or Free (scraping) |
| LinkedIn | Chrome Extension (user session) | No server-side alternative | User's browser cookies | Free |

---

## 7. Safe Link Workflow

This is a unique feature identified by Gemini's research — no competitor addresses this.

### The "Strategic Engagement Loop"

**Problem:** Links are the #1 spam trigger across all platforms. Dropping a link in a Reddit comment without context = instant removal or shadowban.

**Solution:** A 7-step safety system:

| Step | Name | What Happens |
|------|------|-------------|
| 1 | **Signal Capture** | Keyword match detected (e.g., "HubSpot alternative" on r/SaaS) |
| 2 | **Context Analysis** | AI reads FULL thread. Checks: genuine request vs sarcasm? Already solved? Someone already recommended us? Hostile thread? |
| 3 | **Trust Score Check** | Evaluate posting account's age, karma, platform standing. Block link posting if trust score below threshold. |
| 4 | **Response Drafting** | RAG generates 3 versions: (a) pure value comment, (b) technical explanation, (c) soft-sell with link. Shows subreddit/platform rules for links. |
| 5 | **Approval Gate** | Human reviews in extension sidebar or Slack. Selects version. Edits if needed. |
| 6 | **Smart Posting** | Extension posts with human-mimicry (random 2-8s typing delay, natural pauses). If link version: suggests posting value comment FIRST, link as follow-up only after OP engages. |
| 7 | **Track & Learn** | UTM-tagged link tracks clicks → signups. AI learns which reply styles convert best per platform. |

### Link Safety Rules Engine

```
IF account_karma < 100 AND subreddit_requires_karma:
    → BLOCK link posting, suggest value-only comment
    
IF thread_sentiment == "hostile" OR "joke":
    → SKIP engagement entirely
    
IF our_product_already_mentioned_in_thread:
    → SKIP to avoid appearing astroturfed
    
IF subreddit_rules_ban_links:
    → Force value-only comment, suggest DM follow-up
    
IF account_age < 30_days:
    → WARNING: high risk of auto-removal. Suggest building karma first.
```

---

## 8. Build Roadmap

### Phase 1: MVP — Weeks 1-6

**Goal:** Ship monitoring + unified inbox + basic reply for Reddit & HN

- [ ] Go backend: HN Firebase API poller + Reddit API poller
- [ ] AI pipeline: Claude API for relevance scoring (1-10) + intent classification (buy signal / complaint / recommendation ask / general discussion)
- [ ] PostgreSQL + pgvector for mention storage + semantic search
- [ ] Next.js dashboard: Unified Intent Inbox with real-time SSE updates
- [ ] Reply-from-dashboard for Reddit (via Reddit API posting with OAuth)
- [ ] Email + Slack notifications with AI-scored priority
- [ ] Basic RAG: user uploads product docs/FAQs → Claude generates context-aware reply drafts
- [ ] UTM link generation for tracking clicks from replies
- [ ] User auth (email/password + OAuth)
- [ ] Stripe integration for payments

### Phase 2: V2 — Weeks 7-12

**Goal:** Add X/Twitter + Chrome extension for LinkedIn + workflow automation

- [ ] X/Twitter monitoring: API v2 for search (or Playwright scraping fallback)
- [ ] Chrome Extension (Manifest V3): LinkedIn feed monitoring using user's session
- [ ] Extension sidebar: AI reply suggestions + one-click post on any platform
- [ ] Human-mimicry posting: random delays, typing simulation, natural mouse patterns
- [ ] Workflow engine: keyword match → AI draft → Slack approval → auto-post queue
- [ ] Team seats with role-based access (monitor-only, reply, admin)
- [ ] Reply templates per platform with variable injection (`{product_name}`, `{user_pain_point}`)
- [ ] "Safe Link" system: trust score checking + multi-step engagement sequences
- [ ] Webhook integration for CRM sync (Zapier/Make compatible)

### Phase 3: V3 — Weeks 13-20

**Goal:** Lead pipeline CRM + analytics + scale features

- [ ] Full lead pipeline: mention → engaged → replied → clicked → signed up → revenue
- [ ] ROI dashboard: per-platform, per-keyword, per-team-member conversion tracking
- [ ] CRM sync via webhooks: HubSpot, Pipedrive, Salesforce
- [ ] A/B testing: compare reply styles (helpful expert vs. soft-sell vs. direct) per platform
- [ ] Thread-aware AI: reads full conversation context before generating reply
- [ ] RAG v2: learns from YOUR successful past comments to improve future suggestions
- [ ] Agency mode: multi-brand workspaces, per-client keyword sets, separate personas
- [ ] API + MCP server for power users

### Phase 4: V4 — Month 6+

**Goal:** Anti-detect layer + open-source core + marketplace

- [ ] Anti-detect browser integration (GoLogin/Multilogin API) for agency multi-account safety
- [ ] Open-source core monitoring engine → Railway template for self-hosted
- [ ] Reply marketplace: community-contributed reply templates rated by conversion
- [ ] Bluesky + Mastodon + Quora + ProductHunt monitoring
- [ ] Video/audio mention detection (YouTube comments, podcast mention alerts)
- [ ] AI persona manager: multiple expert accounts, each with own RAG voice + posting schedule

---

## 9. Pricing Strategy

### Tier Structure

| Tier | Price | Target | Key Features |
|------|-------|--------|-------------|
| **Starter** | $0/forever | Solo founders testing | 3 keywords, HN + Reddit only, 50 mentions/mo, email alerts |
| **Solo** | $29/mo | Indie hackers | 10 keywords, all 4 platforms, 500 mentions/mo, AI scoring, reply-from-dashboard (Reddit + X), RAG drafts, UTM tracking |
| **Growth** | $69/mo ⭐ | Startups & teams | 25 keywords, 2K mentions/mo, Chrome extension (LinkedIn), workflows + approval gates, 5 seats, lead pipeline, Slack/Discord, CRM webhook |
| **Scale** | $199/mo | Agencies & growing co's | Unlimited keywords, 15K mentions/mo, ROI dashboard, A/B testing, 15 seats, multi-brand, API access |

### Pricing Rationale

- **Octolens:** $15-208/mo for alerts + AI scoring only (no reply, no tracking)
- **ForumScout:** $49/mo for alerts + basic auto-reply (no dashboard reply, no tracking)
- **Our $69 Growth tier:** alerts + AI scoring + reply-from-dashboard + RAG drafts + workflows + lead pipeline + CRM sync

**We deliver 3x the value at the same price point as ForumScout.** The "reply from dashboard" feature alone justifies the premium over Octolens.

---

## 10. Go-To-Market Playbook

| Timeline | Action | Detail |
|----------|--------|--------|
| **Pre-launch** | Dogfood | Use your OWN tool to monitor keywords like "social listening", "lead gen tool", "Reddit monitoring" across HN/Reddit. Reply to every relevant post manually — this IS your marketing. |
| **Week 1** | Show HN + Product Hunt | Launch on HN "Show HN" (you know the culture) and PH same week. Position as "the first tool that lets you REPLY from the dashboard, not just get alerts." |
| **Week 2-3** | Founder outreach | Find 20 indie hackers building in public. Offer free 60-day access. Goal: 10 case studies showing "I got X leads from social replies using LeadEcho." |
| **Month 2** | Content engine | Write comparison posts: "LeadEcho vs Octolens vs ForumScout." Create Railway template for self-hosted open-source core. Publish monitoring engine on GitHub. |
| **Month 3** | Affiliate + community | Launch 30% recurring affiliate program targeting YouTube creators and marketing newsletter authors. Start Discord community for social selling tips. |
| **Month 4+** | Agency tier | Build agency features based on early feedback. Target marketing agencies managing 5+ client social presences. $199/mo × multiple clients = real revenue. |

### Distribution Channels (Ranked by Expected ROI)

1. **Self-referential marketing** — Use LeadEcho to find and engage with people discussing social selling tools. Meta, but extremely effective.
2. **Hacker News** — Show HN launch + genuine participation in relevant threads
3. **Reddit** — r/SaaS, r/startups, r/Entrepreneur, r/marketing
4. **Product Hunt** — Launch with case studies and real screenshots
5. **SEO/Content** — Comparison pages rank well and convert buyers
6. **Open-source core on GitHub** — Developer trust + organic discovery
7. **YouTube creators** — Social selling tutorial space is growing fast
8. **Affiliate program** — Marketing newsletter authors love recurring commissions

---

## 11. Revenue Projections & Risks

### Revenue Projections (Conservative)

| Month | Free Users | Solo ($29) | Growth ($69) | Scale ($199) | MRR |
|-------|-----------|------------|--------------|--------------|-----|
| 1 | 50 | 10 | 0 | 0 | $290 |
| 2 | 120 | 20 | 5 | 0 | $925 |
| 3 | 250 | 50 | 30 | 2 | $3,918 |
| 4 | 400 | 70 | 45 | 5 | $5,130 |
| 5 | 600 | 80 | 60 | 8 | $7,952 |
| 6 | 800 | 100 | 80 | 12 | $10,788 |

**Target: $10K MRR by month 6**

### Key Risks & Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| **X API cost** ($100/mo minimum) | Medium | Start with Playwright scraping. Upgrade to API when revenue covers it. |
| **Reddit Responsible Builder Policy** | High | Apply for OAuth approval during Week 1 of development. Use `.json` endpoint fallback. |
| **LinkedIn anti-bot detection** | High | Chrome extension approach ONLY. Never server-side. Human-in-the-loop mandatory. |
| **Platform TOS changes** | Medium | Build modular architecture. Each platform scraper is swappable. |
| **AI reply quality concerns** | Medium | RAG with user's own docs. Human approval gate. Never auto-post without review. |
| **Competitor response** (Octolens/ForumScout add reply features) | Medium | Speed advantage: ship reply-from-dashboard + safe link workflow first. Build switching costs via RAG training data and lead pipeline history. |
| **User account bans** | High | Safe Link workflow, trust scoring, human-mimicry delays, mandatory approval gates. Never expose users to ban risk without explicit warnings. |

### Cost Structure (Month 1-3)

| Item | Monthly Cost |
|------|-------------|
| Railway (Go services + PG + Redis) | $25-50 |
| Claude API (relevance scoring + RAG replies) | $50-200 |
| X API Basic (if using) | $100 |
| Domain + email | $10 |
| Stripe fees (2.9% + $0.30) | Variable |
| **Total infrastructure** | **~$200-350/mo** |

Breakeven at ~5-10 paying users. Very lean.

---

## Summary: Why This Wins

1. **Reply-from-dashboard is a 0→1 feature** — 15+ competitors analyzed, ZERO let you reply across Reddit, HN, X, and LinkedIn from one inbox. This alone is a Product Hunt launch story.

2. **RAG-trained voice is your MyClone superpower** — You've already built RAG with pgvector. Applying it to social replies is a natural extension nobody else has.

3. **"Safe Link" workflow has no competition** — Gemini's unique insight: a system that scores link relevance, checks trust, and suggests multi-step engagement is completely novel.

4. **Go + Railway = your speed advantage** — 2 years of Go + deep Railway knowledge = MVP in 4-6 weeks. Most competitors are small Python/Node teams.

5. **The pricing gap is massive** — Enterprise tools cost $5K+/mo. Indie tools charge $15-49 for alerts-only. $69/mo for the full loop (alerts + reply + tracking) is a category-creating price point.

---

*Research synthesized from: Claude web search (8 competitors, market data), Gemini deep research (platform-specific API/scraping analysis, anti-detect infrastructure, regulatory landscape), and Grok analysis (7 additional competitors, tech feasibility assessment). February 2026.*

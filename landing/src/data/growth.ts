// Programmatic SEO pages (feature: growth). Every competitor claim comes from
// docs/competitive-analysis-snitchfeed-treg.md and is dated; edit facts here, not in templates.

export const AS_OF = 'Nov 2026';

export interface Row {
  feature: string;
  us: string;
  them: string;
}

export interface Competitor {
  slug: string;
  name: string;
  site: string;
  summary: string; // neutral one-liner about what they do
  rows: Row[]; // empty = we have not verified their feature set yet
  credit: string[]; // what they do well, stated plainly
}

export interface Platform {
  slug: string;
  name: string;
  how: string; // how LeadEcho fetches it
  signals: string[];
  queries: string[];
}

// Shared closed-loop pitch, used on every page.
export const LOOP = [
  { step: 'Reply', text: 'A draft grounded in your own knowledge base, with a UTM link already inserted.' },
  { step: 'Posted', text: 'Post from the Chrome extension in one click; LeadEcho records where and when.' },
  { step: 'Clicked', text: 'Each reply gets a tracked link, so you see which thread sent which visitor.' },
  { step: 'Revenue', text: 'Conversions post back to the lead and sync to HubSpot or Attio.' },
];

export const PILLARS = [
  { title: 'Closed loop to revenue', text: 'Reply, posted, clicked, revenue. Attribution is built in, not an export.' },
  { title: 'CRM, CSV and webhooks on every plan', text: 'Native HubSpot and Attio sync, CSV export and webhooks are not gated to a higher tier.' },
  { title: 'Faster cadence', text: 'Hourly-class scanning instead of multi-hour defaults, so you reply while the thread is still live.' },
  { title: 'Open source and BYOK', text: 'MIT licensed, self-hostable, and AI calls run on your own model keys.' },
];

export const competitors: Competitor[] = [
  {
    slug: 'snitchfeed',
    name: 'SnitchFeed',
    site: 'https://snitchfeed.com',
    summary: 'a social listening tool that monitors Reddit, X, LinkedIn, Bluesky and Hacker News for brand, competitor and buying-intent mentions.',
    credit: [
      'An agent-led setup that reads your site, writes boolean queries and live-tests them per platform before proposing a listener.',
      'An in-app AI chat that re-triages the feed.',
      'An MCP server and a copy-paste prompt library for Claude and ChatGPT.',
      'A 13-tag intent taxonomy (buying intent, comparison, seeking alternative, pain point and more).',
    ],
    rows: [
      { feature: 'Platforms', us: 'Reddit, HN, X, LinkedIn, Bluesky, Dev.to, Lobsters, Indie Hackers, Threads, Google News', them: 'Reddit, X, LinkedIn, Bluesky, HN' },
      { feature: 'Native CRM sync', us: 'HubSpot and Attio, built in', them: 'None native; HubSpot via webhook or Zapier' },
      { feature: 'Webhooks and CSV export', us: 'Every plan', them: 'Pro plan and up' },
      { feature: 'Reply drafting', us: 'Grounded in your knowledge base, available from the first lead', them: 'Available after you create a Response Profile' },
      { feature: 'Reply posting', us: 'One click from the Chrome extension', them: 'Not offered' },
      { feature: 'UTM attribution to revenue', us: 'Built in (reply, posted, clicked, revenue)', them: 'No outcome or revenue metric in analytics' },
      { feature: 'Default scan cadence', us: 'Hourly-class', them: 'Reddit 12h, LinkedIn 12h, HN 6h, X 4h (defaults)' },
      { feature: 'Pricing', us: 'Free tier, paid plans, or self-host free', them: '$47 / $95 / $399 credit plans' },
      { feature: 'MCP server for agents', us: 'Yes', them: 'Yes' },
      { feature: 'Self-host', us: 'MIT licensed, Docker Compose', them: 'Not offered' },
    ],
  },
  {
    slug: 'octolens',
    name: 'Octolens',
    site: 'https://octolens.com',
    summary: 'a social listening tool for tracking brand and keyword mentions across communities.',
    credit: [],
    rows: [],
  },
];

export const platforms: Platform[] = [
  {
    slug: 'reddit',
    name: 'Reddit',
    how: 'LeadEcho reads new posts in the subreddits you choose through the public Reddit JSON API and scores every post against your pain points with semantic matching, not just keyword hits.',
    signals: ['"What do you use for…" recommendation requests', 'Complaints about a competitor', 'People describing the exact pain your product removes'],
    queries: ['"alternative to" AND (your competitor)', '"looking for a tool" AND (your category)', '"how do you handle" AND (your pain point)'],
  },
  {
    slug: 'x',
    name: 'X (Twitter)',
    how: 'On hosted plans LeadEcho fetches X search through a metered data provider, with no cookie to paste. Self-hosters can use their own browser session instead.',
    signals: ['Founders asking for tool recommendations', 'Threads comparing two products in your category', 'Frustrated posts tagging a competitor'],
    queries: ['"any recommendations" (your category)', '"switching from" (your competitor)', '"is there a tool that" (your job-to-be-done)'],
  },
  {
    slug: 'linkedin',
    name: 'LinkedIn',
    how: 'On hosted plans LeadEcho searches LinkedIn posts through a metered data provider. Self-hosters can use their own browser session. Matched authors become leads you can push to your CRM.',
    signals: ['Buyers asking their network for vendor suggestions', 'Posts about evaluating or replacing a tool', 'Hiring posts that reveal a new initiative'],
    queries: ['"recommend a" (your category)', '"we are evaluating" (your category)', '"replacing" (your competitor)'],
  },
  {
    slug: 'hacker-news',
    name: 'Hacker News',
    how: 'LeadEcho searches Hacker News stories and comments through the free public Algolia API, then scores each hit for intent.',
    signals: ['Ask HN threads looking for tools', 'Show HN launches in your space', 'Comment threads comparing approaches'],
    queries: ['"Ask HN" (your category)', '(your competitor) alternative', '(your pain point)'],
  },
  {
    slug: 'bluesky',
    name: 'Bluesky',
    how: 'LeadEcho searches Bluesky through the free public AT Protocol search and scores matches alongside your other sources.',
    signals: ['Developers and founders asking for recommendations', 'Posts about leaving a tool', 'Discussion of the problem your product solves'],
    queries: ['"recommend" (your category)', '(your competitor) alternative', '"anyone using" (your category)'],
  },
];

export type PageKind = 'compare' | 'alternatives' | 'monitor';

export interface GrowthPage {
  section: PageKind;
  slug: string;
  title: string;
  label: string;
}

export const growthPages: GrowthPage[] = [
  ...competitors.flatMap((c) => [
    { section: 'compare' as const, slug: c.slug, title: `LeadEcho vs ${c.name}`, label: `vs ${c.name}` },
    { section: 'alternatives' as const, slug: c.slug, title: `${c.name} alternative`, label: `${c.name} alternative` },
  ]),
  ...platforms.map((p) => ({ section: 'monitor' as const, slug: p.slug, title: `Monitor ${p.name} for buying intent`, label: `Monitor ${p.name}` })),
];

export const SITE = 'https://leadecho.app';

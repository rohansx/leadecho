-- +goose Up

-- Dev workspace
INSERT INTO workspaces (id, clerk_org_id, name, slug, tier)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'org_dev_local',
    'LeadEcho Dev',
    'leadecho-dev',
    'growth'
) ON CONFLICT (id) DO NOTHING;

-- Dev user
INSERT INTO users (id, clerk_user_id, workspace_id, email, name, role)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'user_dev_local',
    '00000000-0000-0000-0000-000000000001',
    'dev@leadecho.app',
    'Dev User',
    'admin'
) ON CONFLICT (id) DO NOTHING;

-- Keywords
INSERT INTO keywords (id, workspace_id, term, platforms) VALUES
    ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000001', 'social listening tool', '{reddit,hackernews,twitter}'),
    ('00000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000001', 'CRM alternative', '{reddit,hackernews}'),
    ('00000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000001', 'lead generation Reddit', '{reddit,twitter}')
ON CONFLICT DO NOTHING;

-- Mentions
INSERT INTO mentions (id, workspace_id, keyword_id, platform, platform_id, url, title, content, author_username, author_profile_url, author_karma, author_account_age_days, relevance_score, intent, conversion_probability, status, platform_metadata, engagement_metrics, keyword_matches, platform_created_at)
VALUES
(
    '00000000-0000-0000-0000-000000000101',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000010',
    'reddit',
    'reddit_abc123',
    'https://reddit.com/r/SaaS/comments/abc123',
    'Looking for a CRM alternative that actually works with social',
    'We''ve been using HubSpot for a year but it completely ignores social selling. Anyone know a tool that monitors Reddit/HN/Twitter for leads and helps you reply? Budget is around $100/mo for a small team.',
    'startup_sarah',
    'https://reddit.com/u/startup_sarah',
    4520,
    730,
    9.2,
    'buy_signal',
    0.85,
    'new',
    '{"subreddit": "r/SaaS", "upvotes": 47, "comment_count": 23}',
    '{"upvotes": 47, "comments": 23, "awards": 1}',
    '{social listening tool,CRM alternative}',
    NOW() - INTERVAL '2 hours'
),
(
    '00000000-0000-0000-0000-000000000102',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000010',
    'hackernews',
    'hn_item_39201',
    'https://news.ycombinator.com/item?id=39201',
    'Ask HN: Best tools for social listening in B2B?',
    'Building a developer tool and want to catch when people mention our problem space on Reddit and HN. Tried Mention.com but it''s too expensive and not great for tech communities. Looking for something more developer-focused.',
    'techfounder',
    'https://news.ycombinator.com/user?id=techfounder',
    892,
    1825,
    8.5,
    'recommendation_ask',
    0.72,
    'new',
    '{"points": 34, "comment_count": 18}',
    '{"points": 34, "comments": 18}',
    '{social listening tool}',
    NOW() - INTERVAL '4 hours'
),
(
    '00000000-0000-0000-0000-000000000103',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000011',
    'twitter',
    'tweet_xyz789',
    'https://twitter.com/growth_mike/status/xyz789',
    NULL,
    'Has anyone compared Syften vs F5Bot for monitoring Reddit mentions? Looking for something with AI-powered reply suggestions that doesn''t feel spammy. @saaspeople',
    '@growth_mike',
    'https://twitter.com/growth_mike',
    NULL,
    NULL,
    7.1,
    'comparison',
    0.55,
    'new',
    '{"retweets": 12, "likes": 38, "replies": 7}',
    '{"retweets": 12, "likes": 38, "replies": 7}',
    '{social listening tool}',
    NOW() - INTERVAL '6 hours'
),
(
    '00000000-0000-0000-0000-000000000104',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000012',
    'reddit',
    'reddit_def456',
    'https://reddit.com/r/Entrepreneur/comments/def456',
    'How are you finding leads on Reddit without being spammy?',
    'I run a B2B SaaS and keep seeing competitors replying to threads on Reddit. Tried doing it manually but it takes hours and feels awkward. Is there a tool that makes this less painful and helps write replies that actually add value?',
    'bootstrapped_ben',
    'https://reddit.com/u/bootstrapped_ben',
    2100,
    540,
    8.8,
    'buy_signal',
    0.78,
    'reviewed',
    '{"subreddit": "r/Entrepreneur", "upvotes": 89, "comment_count": 45}',
    '{"upvotes": 89, "comments": 45, "awards": 3}',
    '{lead generation Reddit}',
    NOW() - INTERVAL '12 hours'
),
(
    '00000000-0000-0000-0000-000000000105',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000010',
    'linkedin',
    'li_post_abc',
    'https://linkedin.com/posts/janemarketer_abc',
    NULL,
    'Frustrated with our current social listening setup. We pay $500/mo for Brandwatch but 90% of what we need is just Reddit and HN monitoring with smart alerts. Anyone know a more focused, affordable option for B2B startups?',
    'janemarketer',
    'https://linkedin.com/in/janemarketer',
    NULL,
    NULL,
    7.8,
    'complaint',
    0.65,
    'new',
    '{"reactions": 24, "comments": 11}',
    '{"reactions": 24, "comments": 11}',
    '{social listening tool}',
    NOW() - INTERVAL '1 day'
),
(
    '00000000-0000-0000-0000-000000000106',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000011',
    'reddit',
    'reddit_ghi789',
    'https://reddit.com/r/startups/comments/ghi789',
    'What tools do you use for inbound lead gen besides Google Ads?',
    'We''re a pre-seed startup and Google Ads CPAs are killing us. Been thinking about monitoring Reddit and Twitter for people asking about our problem space and replying with helpful content. Is this a thing? What tools exist for this?',
    'earlystage_emma',
    'https://reddit.com/u/earlystage_emma',
    380,
    180,
    6.5,
    'general',
    0.40,
    'new',
    '{"subreddit": "r/startups", "upvotes": 23, "comment_count": 34}',
    '{"upvotes": 23, "comments": 34}',
    '{CRM alternative,lead generation Reddit}',
    NOW() - INTERVAL '2 days'
)
ON CONFLICT DO NOTHING;

-- Leads
INSERT INTO leads (id, workspace_id, mention_id, stage, contact_name, contact_email, company, username, platform, profile_url, estimated_value, notes, tags)
VALUES
(
    '00000000-0000-0000-0000-000000000201',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000101',
    'qualified',
    'Sarah Chen',
    'sarah@startupco.io',
    'StartupCo',
    'startup_sarah',
    'reddit',
    'https://reddit.com/u/startup_sarah',
    1200,
    'High-intent buyer. Team of 5, budget $100/mo. Currently using HubSpot.',
    '{saas,high-intent,hubspot-user}'
),
(
    '00000000-0000-0000-0000-000000000202',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000102',
    'engaged',
    NULL,
    NULL,
    NULL,
    'techfounder',
    'hackernews',
    'https://news.ycombinator.com/user?id=techfounder',
    800,
    'Developer tool builder. Replied on HN thread, showed interest.',
    '{developer,hn-lead}'
),
(
    '00000000-0000-0000-0000-000000000203',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000104',
    'prospect',
    'Ben Torres',
    NULL,
    NULL,
    'bootstrapped_ben',
    'reddit',
    'https://reddit.com/u/bootstrapped_ben',
    600,
    'Bootstrapped founder. Looking for automation. Good fit.',
    '{bootstrapped,automation-seeker}'
),
(
    '00000000-0000-0000-0000-000000000204',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000105',
    'prospect',
    'Jane Martinez',
    'jane@marketingfirm.co',
    'Marketing Firm Co',
    'janemarketer',
    'linkedin',
    'https://linkedin.com/in/janemarketer',
    2400,
    'Currently paying $500/mo for Brandwatch. Big potential savings pitch.',
    '{enterprise,brandwatch-user,high-value}'
),
(
    '00000000-0000-0000-0000-000000000205',
    '00000000-0000-0000-0000-000000000001',
    NULL,
    'converted',
    'Alex Rivera',
    'alex@growthio.com',
    'Growth.io',
    'alexgrowth',
    'twitter',
    'https://twitter.com/alexgrowth',
    960,
    'Signed up for Growth plan. Converted from Twitter DM outreach.',
    '{converted,growth-plan}'
),
(
    '00000000-0000-0000-0000-000000000206',
    '00000000-0000-0000-0000-000000000001',
    NULL,
    'lost',
    'David Kim',
    'david@bigcorp.com',
    'BigCorp',
    'davidkim_tech',
    'linkedin',
    'https://linkedin.com/in/davidkim',
    5000,
    'Went with enterprise Brandwatch. Too early for us.',
    '{enterprise,lost-to-competitor}'
)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM leads WHERE workspace_id = '00000000-0000-0000-0000-000000000001';
DELETE FROM mentions WHERE workspace_id = '00000000-0000-0000-0000-000000000001';
DELETE FROM keywords WHERE workspace_id = '00000000-0000-0000-0000-000000000001';
DELETE FROM users WHERE workspace_id = '00000000-0000-0000-0000-000000000001';
DELETE FROM workspaces WHERE id = '00000000-0000-0000-0000-000000000001';

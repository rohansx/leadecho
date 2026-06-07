import { test, expect, type Page } from "@playwright/test";

/**
 * LeadEcho "analytics" feature — analytics charts / breakdown sections.
 *
 * Route: /_dashboard/analytics.tsx, rendered at /app/analytics.
 *
 * Reads (all GET, all return JSON):
 *   GET /api/v1/analytics/overview            -> OverviewStats (object of 6 int counts)
 *   GET /api/v1/analytics/mentions-per-day    -> { day: string; count: number }[]
 *   GET /api/v1/analytics/mentions-per-platform-> { platform: string; count: number }[]
 *   GET /api/v1/analytics/mentions-per-intent -> { intent: string; count: number }[]
 *   GET /api/v1/analytics/conversion-funnel   -> { stage: string; count: number }[]
 *   GET /api/v1/analytics/top-keywords        -> { term: string; mention_count: number }[]
 *   GET /api/v1/utm-links                     -> UTMLink[]
 *
 * NOTE on the DOM: despite the feature brief mentioning "recharts or similar",
 * the route renders NO charting library. It draws bespoke CSS bar charts (a
 * muted track div with an inner bg-primary / bg-green-500 div whose width is a
 * percentage). So assertions target the rendered text + structure, never an
 * <svg> recharts container.
 *
 * Section structure (CardTitle renders an <h3>, so getByRole("heading") works):
 *   - Page <h2> "Analytics".
 *   - 6 KPI cards (covered more deeply by overview.spec.ts; here we only smoke them).
 *   - "Mentions by Platform" card: empty-state "No data yet." OR one row per
 *     platform: a Badge with the platform name + a count <span> + a bar.
 *   - "Mentions by Intent" card: empty-state "No data yet." OR one row per intent
 *     (label mapped via intentLabels, fallback to raw intent) + count + bar.
 *   - "Conversion Funnel <rate>% rate" card: ALWAYS renders all 5 stage rows
 *     (prospect, qualified, engaged, converted, lost) — each a Badge + a bar whose
 *     inner div shows the count. Empty-state line "No leads yet." also shows when
 *     the funnel payload is empty (it renders ABOVE the always-present stage rows).
 *   - "Top Keywords" card: empty-state "No keywords tracked yet." OR a numbered
 *     list of "<term>" + "<n> mentions" Badge.
 *   - "UTM Tracking Links" card (id="utm-links").
 *
 * Seed (from the authenticated storageState): a complete-onboarding user with a
 * monitoring profile, keywords "product analytics" / "funnel tracking" /
 * "user retention", and ~59 seeded mentions. So platform/intent/top-keyword and
 * mentions_30d data are expected to be populated (non-empty / non-zero). Lead and
 * conversion data MAY be sparse, so funnel/converted assertions tolerate zero.
 *
 * IDEMPOTENCY: the only test that mutates state creates a UTM link with a unique
 * epoch-ms suffix and deletes it (UI delete is the behavior under test, with an
 * API safety-net in finally). Empty-state branches are exercised via per-page
 * route interception (page.route) so no seeded data is ever destroyed.
 */

const ANALYTICS_URL = "/app/analytics";

const FUNNEL_STAGES = ["prospect", "qualified", "engaged", "converted", "lost"];

const SECTION_TITLES = [
  "Mentions by Platform",
  "Mentions by Intent",
  "Top Keywords",
  "UTM Tracking Links",
];

type CountRow = { count: number };
type PlatformRow = { platform: string; count: number };
type IntentRow = { intent: string; count: number };
type StageRow = { stage: string; count: number };
type KeywordRow = { term: string; mention_count: number };

// Fail the test if the page logs an uncaught error / unhandled rejection while
// the analytics queries resolve — "renders without error" is the core contract.
function trackPageErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  return errors;
}

test.describe("analytics / charts & breakdown sections", () => {
  test("page loads authenticated and renders the Analytics heading + every section", async ({
    page,
  }) => {
    const errors = trackPageErrors(page);

    const resp = await page.goto(ANALYTICS_URL);
    expect(resp?.status(), "analytics HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Analytics", exact: true }),
    ).toBeVisible();

    // Every breakdown / management section heading renders (CardTitle = <h3>).
    for (const title of SECTION_TITLES) {
      await expect(
        page.getByRole("heading", { name: new RegExp(title) }),
        `section heading "${title}" visible`,
      ).toBeVisible();
    }

    // The Conversion Funnel heading carries a trailing "<rate>% rate" badge, so
    // match loosely on the prefix.
    await expect(
      page.getByRole("heading", { name: /Conversion Funnel/ }),
      "conversion funnel heading visible",
    ).toBeVisible();

    // Give react-query a beat to settle, then assert the page never threw.
    await page.waitForLoadState("networkidle");
    expect(errors, `page errors: ${errors.join(" | ")}`).toEqual([]);
  });

  test("all six analytics endpoints honour their documented JSON contracts", async ({
    page,
  }) => {
    // ── overview: object of six non-negative integers ──
    const ovRes = await page.request.get("/api/v1/analytics/overview");
    expect(ovRes.ok(), "/analytics/overview ok").toBeTruthy();
    const overview = (await ovRes.json()) as Record<string, number>;
    for (const key of [
      "mentions_30d",
      "mentions_new",
      "total_leads",
      "converted_leads",
      "replies_posted",
      "active_keywords",
    ]) {
      expect(overview, `overview has "${key}"`).toHaveProperty(key);
      expect(
        Number.isInteger(overview[key]),
        `overview.${key} is an integer (got ${overview[key]})`,
      ).toBeTruthy();
      expect(overview[key], `overview.${key} >= 0`).toBeGreaterThanOrEqual(0);
    }

    // ── mentions-per-day: array of { day:string, count:int } ──
    // (this endpoint exists in the API but is NOT consumed by the route; cover
    //  the contract at the API level so a regression is still caught.)
    const dayRes = await page.request.get("/api/v1/analytics/mentions-per-day");
    expect(dayRes.ok(), "/mentions-per-day ok").toBeTruthy();
    const days = (await dayRes.json()) as { day: string; count: number }[];
    expect(Array.isArray(days), "mentions-per-day is an array").toBeTruthy();
    for (const d of days) {
      expect(typeof d.day, "day is a string").toBe("string");
      expect(Number.isInteger(d.count), "day.count is an integer").toBeTruthy();
    }

    // ── mentions-per-platform ──
    const platRes = await page.request.get(
      "/api/v1/analytics/mentions-per-platform",
    );
    expect(platRes.ok(), "/mentions-per-platform ok").toBeTruthy();
    const platforms = (await platRes.json()) as PlatformRow[];
    expect(Array.isArray(platforms), "platforms is an array").toBeTruthy();
    for (const p of platforms) {
      expect(typeof p.platform, "platform is a string").toBe("string");
      expect(Number.isInteger(p.count), "platform.count is int").toBeTruthy();
      expect(p.count, "platform.count >= 0").toBeGreaterThanOrEqual(0);
    }

    // ── mentions-per-intent ──
    const intentRes = await page.request.get(
      "/api/v1/analytics/mentions-per-intent",
    );
    expect(intentRes.ok(), "/mentions-per-intent ok").toBeTruthy();
    const intents = (await intentRes.json()) as IntentRow[];
    expect(Array.isArray(intents), "intents is an array").toBeTruthy();
    for (const i of intents) {
      expect(typeof i.intent, "intent is a string").toBe("string");
      expect(Number.isInteger(i.count), "intent.count is int").toBeTruthy();
    }

    // ── conversion-funnel ──
    const funnelRes = await page.request.get(
      "/api/v1/analytics/conversion-funnel",
    );
    expect(funnelRes.ok(), "/conversion-funnel ok").toBeTruthy();
    const funnel = (await funnelRes.json()) as StageRow[];
    expect(Array.isArray(funnel), "funnel is an array").toBeTruthy();
    for (const f of funnel) {
      expect(typeof f.stage, "stage is a string").toBe("string");
      expect(Number.isInteger(f.count), "stage.count is int").toBeTruthy();
    }

    // ── top-keywords ──
    const kwRes = await page.request.get("/api/v1/analytics/top-keywords");
    expect(kwRes.ok(), "/top-keywords ok").toBeTruthy();
    const keywords = (await kwRes.json()) as KeywordRow[];
    expect(Array.isArray(keywords), "top-keywords is an array").toBeTruthy();
    for (const k of keywords) {
      expect(typeof k.term, "kw.term is a string").toBe("string");
      expect(
        Number.isInteger(k.mention_count),
        "kw.mention_count is int",
      ).toBeTruthy();
    }

    // Seeded fixture guarantees populated mentions → at least one platform row.
    expect(
      overview.mentions_30d,
      "seeded mentions in last 30d are non-zero",
    ).toBeGreaterThan(0);
    expect(
      platforms.length,
      "seeded mentions produce >=1 platform breakdown row",
    ).toBeGreaterThan(0);
  });

  test("Mentions-by-Platform chart renders a bar + count for each API platform row", async ({
    page,
  }) => {
    const res = await page.request.get(
      "/api/v1/analytics/mentions-per-platform",
    );
    const platforms = (await res.json()) as PlatformRow[];
    test.skip(platforms.length === 0, "no seeded platform data to assert on");

    await page.goto(ANALYTICS_URL);

    // Scope to the platform card by its heading's nearest Card ancestor.
    const card = page
      .getByRole("heading", { name: "Mentions by Platform", exact: true })
      .locator("xpath=ancestor::div[contains(@class,'border-2')][1]");
    await expect(card).toBeVisible();

    // Empty-state must NOT be shown when we have data.
    await expect(card.getByText("No data yet.", { exact: true })).toHaveCount(0);

    for (const p of platforms) {
      // Platform name renders inside a Badge in this card.
      await expect(
        card.getByText(p.platform, { exact: true }).first(),
        `platform badge "${p.platform}"`,
      ).toBeVisible();
      // The numeric count for the row renders as text.
      await expect(
        card.getByText(String(p.count), { exact: true }).first(),
        `platform count "${p.count}"`,
      ).toBeVisible();
    }

    // Each platform row draws a bar track (h-2 rounded-full) — at least one exists.
    expect(
      await card.locator("div.h-2.rounded-full").count(),
      "platform bar tracks rendered",
    ).toBeGreaterThanOrEqual(platforms.length);
  });

  test("Mentions-by-Intent chart renders each intent (mapped label) with its count", async ({
    page,
  }) => {
    const res = await page.request.get(
      "/api/v1/analytics/mentions-per-intent",
    );
    const intents = (await res.json()) as IntentRow[];
    test.skip(intents.length === 0, "no seeded intent data to assert on");

    // The route maps raw intent keys to friendly labels; mirror that here.
    const intentLabels: Record<string, string> = {
      buy_signal: "Buy Signal",
      complaint: "Complaint",
      recommendation_ask: "Recommendation",
      comparison: "Comparison",
      general: "General",
    };

    await page.goto(ANALYTICS_URL);
    const card = page
      .getByRole("heading", { name: "Mentions by Intent", exact: true })
      .locator("xpath=ancestor::div[contains(@class,'border-2')][1]");
    await expect(card).toBeVisible();
    await expect(card.getByText("No data yet.", { exact: true })).toHaveCount(0);

    for (const i of intents) {
      const label = intentLabels[i.intent] ?? i.intent;
      await expect(
        card.getByText(label, { exact: true }).first(),
        `intent label "${label}"`,
      ).toBeVisible();
    }
  });

  test("Conversion Funnel always renders all five stage rows (sparse-data tolerant)", async ({
    page,
  }) => {
    // Funnel payload may legitimately be empty/sparse if leads aren't seeded, but
    // the route ALWAYS renders the five fixed stage rows regardless of data.
    const res = await page.request.get("/api/v1/analytics/conversion-funnel");
    const funnel = (await res.json()) as StageRow[];

    await page.goto(ANALYTICS_URL);
    const card = page
      .getByRole("heading", { name: /Conversion Funnel/ })
      .locator("xpath=ancestor::div[contains(@class,'border-2')][1]");
    await expect(card).toBeVisible();

    // The "<rate>% rate" badge in the heading reflects converted/total. When
    // total_leads is 0 the route emits "0" (no decimal); otherwise "<n>.<d>".
    await expect(
      card.getByText(/%\s*rate/),
      "conversion rate badge present",
    ).toBeVisible();

    // All five stage badges render, each paired with its count (defaulting to 0).
    for (const stage of FUNNEL_STAGES) {
      const apiRow = funnel.find((f) => f.stage === stage);
      const expectedCount = apiRow?.count ?? 0;
      await expect(
        card.getByText(stage, { exact: true }).first(),
        `funnel stage badge "${stage}"`,
      ).toBeVisible();
      // The count is rendered inside the stage's bar; assert it appears in the card.
      await expect(
        card.getByText(String(expectedCount)).first(),
        `funnel stage "${stage}" count "${expectedCount}" shown`,
      ).toBeVisible();
    }
  });

  test("Top Keywords list surfaces a seeded keyword term with its mention count", async ({
    page,
  }) => {
    const res = await page.request.get("/api/v1/analytics/top-keywords");
    const keywords = (await res.json()) as KeywordRow[];

    await page.goto(ANALYTICS_URL);
    const card = page
      .getByRole("heading", { name: "Top Keywords", exact: true })
      .locator("xpath=ancestor::div[contains(@class,'border-2')][1]");
    await expect(card).toBeVisible();

    if (keywords.length === 0) {
      // Sparse/empty branch: the placeholder must render.
      await expect(
        card.getByText("No keywords tracked yet.", { exact: true }),
        "top-keywords empty-state",
      ).toBeVisible();
      return;
    }

    // Populated branch: empty-state hidden, every API term + "<n> mentions" shows.
    await expect(
      card.getByText("No keywords tracked yet.", { exact: true }),
    ).toHaveCount(0);

    for (const kw of keywords) {
      await expect(
        card.getByText(kw.term, { exact: true }).first(),
        `top-keyword term "${kw.term}"`,
      ).toBeVisible();
      await expect(
        card.getByText(`${kw.mention_count} mentions`, { exact: true }).first(),
        `top-keyword "${kw.term}" count badge`,
      ).toBeVisible();
    }
  });

  test("empty / sparse data renders graceful placeholders, not an error (mocked)", async ({
    page,
  }) => {
    const errors = trackPageErrors(page);

    // Force every breakdown endpoint to return an empty array for THIS page only,
    // so the seeded fixture is untouched but we still exercise the empty branches.
    const emptyEndpoints = [
      "**/api/v1/analytics/mentions-per-platform",
      "**/api/v1/analytics/mentions-per-intent",
      "**/api/v1/analytics/conversion-funnel",
      "**/api/v1/analytics/top-keywords",
    ];
    for (const ep of emptyEndpoints) {
      await page.route(ep, async (route) => {
        if (route.request().method() === "GET") {
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: "[]",
          });
          return;
        }
        await route.continue();
      });
    }
    // Zero out the overview too, so the conversion rate falls into the "0" branch.
    await page.route("**/api/v1/analytics/overview", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            mentions_30d: 0,
            mentions_new: 0,
            total_leads: 0,
            converted_leads: 0,
            replies_posted: 0,
            active_keywords: 0,
          }),
        });
        return;
      }
      await route.continue();
    });

    await page.goto(ANALYTICS_URL);

    // Platform + intent cards show "No data yet."; both placeholders are present.
    await expect(
      page.getByText("No data yet.", { exact: true }),
      "platform/intent empty-state placeholders",
    ).toHaveCount(2, { timeout: 15_000 });

    // Funnel empty-state line.
    await expect(
      page.getByText("No leads yet.", { exact: true }),
      "funnel empty-state",
    ).toBeVisible();

    // Top-keywords empty-state line.
    await expect(
      page.getByText("No keywords tracked yet.", { exact: true }),
      "top-keywords empty-state",
    ).toBeVisible();

    // The funnel STILL renders all five fixed stage rows even with no data.
    const card = page
      .getByRole("heading", { name: /Conversion Funnel/ })
      .locator("xpath=ancestor::div[contains(@class,'border-2')][1]");
    for (const stage of FUNNEL_STAGES) {
      await expect(
        card.getByText(stage, { exact: true }).first(),
        `funnel stage "${stage}" still rendered when empty`,
      ).toBeVisible();
    }

    // The conversion rate badge degrades to "0% rate" (total_leads === 0 branch).
    await expect(
      card.getByText("0% rate", { exact: true }),
      "conversion rate is 0% with zero leads",
    ).toBeVisible();

    // No uncaught errors despite the all-empty payloads.
    await page.waitForLoadState("networkidle");
    expect(errors, `page errors: ${errors.join(" | ")}`).toEqual([]);

    for (const ep of emptyEndpoints) await page.unroute(ep);
    await page.unroute("**/api/v1/analytics/overview");
  });

  test("UTM tracking link create → render → delete round-trips in the UI (self-cleaning)", async ({
    page,
  }) => {
    await page.goto(ANALYTICS_URL);

    const uniqueSuffix = Date.now();
    const source = `e2e-analytics-${uniqueSuffix}`;
    const campaign = `camp-${uniqueSuffix}`;
    const destination = `https://example.com/analytics-e2e/${uniqueSuffix}`;
    let createdId: string | null = null;

    const utmCard = page.locator("#utm-links");
    await expect(utmCard).toBeVisible();

    try {
      // Open the inline create form and fill destination + source (+ optional campaign).
      await utmCard.getByRole("button", { name: "New Link" }).click();
      await utmCard
        .getByPlaceholder("Destination URL (e.g. https://example.com/page)")
        .fill(destination);
      await utmCard.getByPlaceholder("Source (e.g. reddit)").fill(source);
      await utmCard
        .getByPlaceholder("Campaign (optional)")
        .fill(campaign);

      await utmCard.getByRole("button", { name: "Create", exact: true }).click();

      // Real behavior: the new row renders its destination, source · campaign, and
      // a generated /r/<code> short link, starting at "0 clicks".
      // Scope to the row div (class border-b); a bare .locator("div") also matches
      // the card wrapper holding every row's Delete button (strict-mode clash).
      const newRow = utmCard
        .locator("div.border-b")
        .filter({ hasText: destination })
        .filter({ hasText: source })
        .first();
      await expect(newRow, "created UTM row visible").toBeVisible({
        timeout: 15_000,
      });
      await expect(
        newRow.getByText(`${source} · ${campaign}`, { exact: true }),
        "source · campaign subtitle",
      ).toBeVisible();
      await expect(
        newRow.getByText(/0 clicks/),
        "new link starts at 0 clicks",
      ).toBeVisible();
      // The short code renders as /r/<code>.
      await expect(
        newRow.getByText(/^\/r\//),
        "short /r/<code> rendered",
      ).toBeVisible();

      // Confirm persistence + capture id for the safety-net cleanup.
      const listRes = await page.request.get("/api/v1/utm-links");
      const links = (await listRes.json()) as {
        id: string;
        utm_source: string;
        destination_url: string;
      }[];
      const created = links.find((l) => l.utm_source === source);
      expect(created, "created UTM link persisted in API").toBeTruthy();
      createdId = created!.id;

      // ── Delete through the UI (the behavior under test) ──
      await newRow.getByRole("button", { name: "Delete" }).click();

      await expect(
        utmCard.getByText(destination, { exact: true }),
        "deleted UTM row removed from list",
      ).toHaveCount(0, { timeout: 15_000 });

      // Confirm deletion via the API.
      const afterRes = await page.request.get("/api/v1/utm-links");
      const after = (await afterRes.json()) as { utm_source: string }[];
      expect(
        after.some((l) => l.utm_source === source),
        "UTM link absent from API after delete",
      ).toBe(false);
      createdId = null;
    } finally {
      // Safety net: remove the link if anything above failed post-creation.
      if (createdId) {
        await page.request.delete(`/api/v1/utm-links/${createdId}`);
      } else {
        const r = await page.request.get("/api/v1/utm-links");
        const leftover = (
          (await r.json()) as { id: string; utm_source: string }[]
        ).find((l) => l.utm_source === source);
        if (leftover)
          await page.request.delete(`/api/v1/utm-links/${leftover.id}`);
      }
    }
  });
});

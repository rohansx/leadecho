import { test, expect } from "@playwright/test";

/**
 * LeadEcho "overview" feature — the dashboard analytics/overview stats page.
 *
 * The overview KPIs live on the `/_dashboard/analytics` route (rendered at
 * /app/analytics). It reads GET /api/v1/analytics/overview and renders six
 * stat cards (Mentions 30d, New/Unread, Total Leads, Converted, Replies Posted,
 * Active Keywords), plus platform/intent/funnel/top-keyword breakdowns and a
 * self-contained UTM tracking links manager.
 *
 * The authenticated storageState provides a logged-in user whose onboarding is
 * complete, with a monitoring profile, the keywords "product analytics",
 * "funnel tracking", "user retention", and ~59 seeded mentions. So these stats
 * are expected to be populated (non-zero) for the keyword-driven values.
 */

const OVERVIEW_URL = "/app/analytics";

// The six KPI stat-card labels exactly as rendered in analytics.tsx.
const STAT_LABELS = [
  "Mentions (30d)",
  "New / Unread",
  "Total Leads",
  "Converted",
  "Replies Posted",
  "Active Keywords",
];

test.describe("overview / analytics stats", () => {
  test("page loads authenticated and renders the Analytics heading", async ({
    page,
  }) => {
    const resp = await page.goto(OVERVIEW_URL);
    expect(resp?.status(), "analytics HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login or onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Analytics", exact: true }),
    ).toBeVisible();
  });

  test("all six KPI stat cards render with numeric values", async ({ page }) => {
    await page.goto(OVERVIEW_URL);

    // Each stat card renders its label text and a sibling number. We locate the
    // card by its (unique) label, then read the value paragraph above it.
    for (const label of STAT_LABELS) {
      const labelNode = page.getByText(label, { exact: true });
      await expect(labelNode, `stat label "${label}" visible`).toBeVisible();

      // The CardContent holds: <Icon/> <p>{value}</p> <p>{label}</p>.
      // The value <p> is the immediately-preceding sibling of the label <p>.
      const card = labelNode.locator(
        "xpath=ancestor::*[contains(@class,'p-4')][1]",
      );
      const value = card.locator("p").first();
      await expect(value, `stat value for "${label}" visible`).toBeVisible();
      await expect(
        value,
        `stat value for "${label}" is an integer`,
      ).toHaveText(/^\d+$/);
    }
  });

  test("overview API contract matches the rendered numbers", async ({
    page,
  }) => {
    // Fetch the raw overview payload through the authenticated request context.
    const res = await page.request.get("/api/v1/analytics/overview");
    expect(res.ok(), "overview endpoint ok").toBeTruthy();
    const stats = (await res.json()) as Record<string, number>;

    // Every documented OverviewStats key is present and a non-negative integer.
    for (const key of [
      "mentions_30d",
      "mentions_new",
      "total_leads",
      "converted_leads",
      "replies_posted",
      "active_keywords",
    ]) {
      expect(stats, `overview has "${key}"`).toHaveProperty(key);
      expect(
        Number.isInteger(stats[key]),
        `"${key}" is an integer (got ${stats[key]})`,
      ).toBeTruthy();
      expect(stats[key], `"${key}" >= 0`).toBeGreaterThanOrEqual(0);
    }

    // Seeded fixture has 3 active keywords and ~59 mentions → these are non-zero.
    expect(stats.active_keywords, "seeded active keywords").toBeGreaterThan(0);
    expect(stats.mentions_30d, "seeded mentions in last 30d").toBeGreaterThan(0);

    // The UI must reflect the API value for Active Keywords (populated state).
    await page.goto(OVERVIEW_URL);
    const akLabel = page.getByText("Active Keywords", { exact: true });
    const akCard = akLabel.locator(
      "xpath=ancestor::*[contains(@class,'p-4')][1]",
    );
    await expect(akCard.locator("p").first()).toHaveText(
      String(stats.active_keywords),
    );
  });

  test("breakdown sections and the conversion funnel render", async ({
    page,
  }) => {
    await page.goto(OVERVIEW_URL);

    // The four breakdown cards (by CardTitle text).
    for (const title of [
      "Mentions by Platform",
      "Mentions by Intent",
      "Top Keywords",
    ]) {
      await expect(
        page.getByText(title, { exact: true }),
        `section "${title}" visible`,
      ).toBeVisible();
    }

    // Conversion Funnel title contains a "<rate>% rate" badge, so match loosely.
    await expect(
      page.getByText(/Conversion Funnel/),
      "conversion funnel section visible",
    ).toBeVisible();

    // The funnel always renders all five lead-stage badges, regardless of data.
    for (const stage of [
      "prospect",
      "qualified",
      "engaged",
      "converted",
      "lost",
    ]) {
      await expect(
        page.getByText(stage, { exact: true }).first(),
        `funnel stage "${stage}" badge`,
      ).toBeVisible();
    }

    // Seeded keywords should surface in the Top Keywords list.
    const topKw = page.getByText("Top Keywords", { exact: true });
    const topKwCard = topKw.locator(
      "xpath=ancestor::*[contains(@class,'gap-0') or self::*][1]",
    );
    // At least one seeded keyword term is expected to appear somewhere on page.
    await expect(
      page
        .getByText("product analytics", { exact: true })
        .or(page.getByText("funnel tracking", { exact: true }))
        .or(page.getByText("user retention", { exact: true }))
        .first(),
      "a seeded keyword term appears",
    ).toBeVisible();
    expect(await topKwCard.count()).toBeGreaterThan(0);
  });

  test("UTM tracking link can be created and deleted (self-cleaning)", async ({
    page,
  }) => {
    await page.goto(OVERVIEW_URL);

    const uniqueSuffix = Date.now();
    const source = `e2e-${uniqueSuffix}`;
    const destination = `https://example.com/overview-e2e/${uniqueSuffix}`;

    const utmCard = page.locator("#utm-links");
    await expect(utmCard).toBeVisible();

    // Open the inline create form.
    await utmCard.getByRole("button", { name: "New Link" }).click();
    await utmCard
      .getByPlaceholder("Destination URL (e.g. https://example.com/page)")
      .fill(destination);
    await utmCard.getByPlaceholder("Source (e.g. reddit)").fill(source);

    // Submit. On success the form closes (react-query invalidates utm-links).
    await utmCard.getByRole("button", { name: "Create" }).click();

    // The new link row renders its destination URL and the unique source label.
    // Scope to the row div (class border-b) — a bare .locator("div") also matches
    // the card wrapper, which holds every row's Delete button (strict-mode clash
    // once more than one link exists).
    const newRow = utmCard
      .locator("div.border-b")
      .filter({ hasText: destination })
      .filter({ hasText: source })
      .first();
    await expect(newRow, "created UTM row visible").toBeVisible({
      timeout: 15_000,
    });
    // Newly created link starts at 0 clicks.
    await expect(newRow.getByText(/0 clicks/)).toBeVisible();

    // Clean up: delete the row we created (the Trash button has title="Delete").
    await newRow.getByRole("button", { name: "Delete" }).click();

    // The row (matched by its unique destination) is gone after deletion.
    await expect(
      utmCard.getByText(destination, { exact: true }),
      "deleted UTM row removed",
    ).toHaveCount(0, { timeout: 15_000 });
  });
});

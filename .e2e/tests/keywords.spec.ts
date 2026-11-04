import { test, expect, type Page, type Locator } from "@playwright/test";

/**
 * LeadEcho "keywords" feature — keyword CRUD.
 *
 * Route: /_dashboard/keywords.tsx, rendered at /app/keywords.
 * Reads:
 *   GET  /api/v1/keywords            -> Keyword[]   (NOT paginated, a bare array)
 * Mutates:
 *   POST   /api/v1/keywords          body { term, platforms, match_type,
 *                                           negative_terms, subreddits } -> Keyword (201)
 *   PUT    /api/v1/keywords/{id}     body { is_active } (+ optional fields) -> Keyword
 *   DELETE /api/v1/keywords/{id}     -> { status: "deleted" }
 *
 * UI shape (from keywords.tsx):
 *   - Page heading <h2> "Keywords" (Text as="h2").
 *   - "Add Keyword" card: an <input placeholder="Enter keyword or phrase...">, an
 *     "Add" <button> (icon + text "Add"), platform toggle buttons (all 7 selected by
 *     default), match-type toggles (broad/exact/phrase, "broad" default), a
 *     "Negative Terms" input, and — because reddit is selected by default — a
 *     "Subreddits" input.
 *   - Submitting (Enter or "Add") fires createKeyword with the selected platforms
 *     (all 7), match_type="broad", parsed negative_terms + subreddits, then clears
 *     the term/negative/subreddit inputs and resets match_type to "broad", and
 *     invalidates ["keywords"] so the list refetches.
 *   - "Active Keywords" card: CardDescription reads "<n> keyword(s) configured", then
 *     either a loading line, an empty-state ("No keywords yet. Add one above to start
 *     monitoring."), or one row per keyword. Each row shows the term (font-medium
 *     truncate span), a match_type Badge, then a per-platform Badge list, optional
 *     r/<subreddit> Badges and a negative-terms line. Each row has exactly two icon
 *     <button>s with NO accessible text: [0] = active/inactive toggle (Power/PowerOff),
 *     [1] = delete (Trash2). We scope to a row by its term and pick buttons by index.
 *
 * Seed (from the authenticated storageState): three keywords already exist for this
 * workspace — "product analytics", "funnel tracking", "user retention".
 *
 * IDEMPOTENCY: every test that creates a keyword uses a unique epoch-ms suffix so reruns
 * never collide, and deletes what it created in a finally block (via UI where that IS the
 * behavior under test, otherwise via the API). The seeded keywords are never mutated.
 */

const KEYWORDS_URL = "/keywords";
const SEEDED = ["product analytics", "funnel tracking", "user retention"];

// Locate a keyword row (the per-keyword wrapper div) by its exact term text. The term
// renders in a font-medium/truncate <span>; the closest ancestor with rounded border-border
// is the row that also holds the toggle + delete buttons.
function keywordRow(page: Page, term: string): Locator {
  return page
    .getByText(term, { exact: true })
    .locator(
      "xpath=ancestor::div[contains(@class,'rounded') and contains(@class,'border-border')][1]",
    );
}

// The two icon buttons in a row have no accessible name; [0] toggles active, [1] deletes.
function deleteButton(row: Locator): Locator {
  return row.getByRole("button").nth(1);
}
function toggleButton(row: Locator): Locator {
  return row.getByRole("button").nth(0);
}

test.describe("keywords / keyword CRUD", () => {
  test("page loads authenticated and renders the Keywords heading", async ({
    page,
  }) => {
    const resp = await page.goto(KEYWORDS_URL);
    expect(resp?.status(), "keywords HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Keywords", exact: true }),
    ).toBeVisible();

    // The add form is present.
    await expect(
      page.getByPlaceholder("Enter keyword or phrase..."),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Add" })).toBeVisible();
  });

  test("seeded keywords render and the header count matches the API", async ({
    page,
  }) => {
    // Read the raw payload through the authenticated request context. /keywords
    // returns a bare array (no pagination envelope).
    const res = await page.request.get("/api/v1/keywords");
    expect(res.ok(), "/keywords endpoint ok").toBeTruthy();
    const list = (await res.json()) as { id: string; term: string }[];
    expect(Array.isArray(list), "keywords payload is an array").toBeTruthy();
    expect(list.length, "fixture seeds at least 3 keywords").toBeGreaterThanOrEqual(3);

    const terms = list.map((k) => k.term);
    for (const t of SEEDED) {
      expect(terms, `seeded keyword "${t}" present in API`).toContain(t);
    }

    await page.goto(KEYWORDS_URL);

    // "Loading..." line disappears once react-query resolves.
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });

    // CardDescription reads "<n> keyword(s) configured" — must equal payload length.
    const plural = list.length !== 1 ? "s" : "";
    await expect(
      page.getByText(`${list.length} keyword${plural} configured`, {
        exact: true,
      }),
      "header count badge matches API",
    ).toBeVisible();

    // Each seeded keyword renders as a row.
    for (const t of SEEDED) {
      await expect(
        keywordRow(page, t),
        `seeded keyword row "${t}" visible`,
      ).toBeVisible();
    }
  });

  test("a seeded keyword row shows its match_type and platform badges", async ({
    page,
  }) => {
    const res = await page.request.get("/api/v1/keywords");
    const list = (await res.json()) as {
      term: string;
      match_type: string;
      platforms: string[];
    }[];
    const sample = list.find((k) => SEEDED.includes(k.term));
    expect(sample, "found a seeded keyword to inspect").toBeTruthy();

    await page.goto(KEYWORDS_URL);
    const row = keywordRow(page, sample!.term);
    await expect(row).toBeVisible();

    // match_type Badge renders the keyword's match_type (falling back to "broad").
    const expectedMatch = sample!.match_type || "broad";
    await expect(
      row.getByText(expectedMatch, { exact: true }),
      `match_type badge "${expectedMatch}"`,
    ).toBeVisible();

    // Each platform on the keyword renders as a Badge inside the row.
    for (const p of sample!.platforms) {
      await expect(
        row.getByText(p, { exact: true }),
        `platform badge "${p}"`,
      ).toBeVisible();
    }

    // The row exposes exactly two action buttons (toggle + delete).
    await expect(row.getByRole("button")).toHaveCount(2);
  });

  test("create a keyword via the UI: it appears in the list, then delete it (self-cleaning)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const term = `e2e keyword ${uniqueSuffix}`;
    let createdId: string | null = null;

    await page.goto(KEYWORDS_URL);
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });

    // Baseline count from the rendered header before we add.
    const beforeRes = await page.request.get("/api/v1/keywords");
    const beforeCount = ((await beforeRes.json()) as unknown[]).length;

    try {
      // Type the term and submit via the "Add" button.
      const input = page.getByPlaceholder("Enter keyword or phrase...");
      await input.fill(term);
      await page.getByRole("button", { name: "Add" }).click();

      // Real behavior: the new keyword row appears after the mutation + refetch.
      const newRow = keywordRow(page, term);
      await expect(newRow, "newly created keyword row visible").toBeVisible({
        timeout: 15_000,
      });

      // The input is cleared on success (onSuccess resets newTerm).
      await expect(input).toHaveValue("");

      // Header count incremented by exactly one.
      await expect(
        page.getByText(`${beforeCount + 1} keywords configured`, {
          exact: true,
        }),
        "header count incremented after create",
      ).toBeVisible({ timeout: 15_000 });

      // The new keyword defaulted to match_type "broad" (UI default) — its badge shows.
      await expect(
        newRow.getByText("broad", { exact: true }),
        "new keyword match_type badge is broad",
      ).toBeVisible();

      // Confirm persistence + capture id via the API.
      const afterCreate = await page.request.get("/api/v1/keywords");
      const afterList = (await afterCreate.json()) as {
        id: string;
        term: string;
        platforms: string[];
        match_type: string;
        is_active: boolean;
      }[];
      const created = afterList.find((k) => k.term === term);
      expect(created, "created keyword persisted in API").toBeTruthy();
      createdId = created!.id;
      // UI default selects every offered platform (the list has grown beyond the
      // original 7); new keyword is active by default.
      expect(created!.platforms.length).toBeGreaterThanOrEqual(7);
      expect(created!.is_active).toBe(true);

      // ── Now DELETE it through the UI (the behavior under test) ──
      // Delete is the second icon button in the row.
      await deleteButton(newRow).click();

      // After deleteMutation + refetch the row is gone from the list.
      await expect(
        keywordRow(page, term),
        "keyword row removed after delete",
      ).toHaveCount(0, { timeout: 15_000 });

      // Header count returns to the baseline.
      await expect(
        page.getByText(
          `${beforeCount} keyword${beforeCount !== 1 ? "s" : ""} configured`,
          { exact: true },
        ),
        "header count back to baseline after delete",
      ).toBeVisible({ timeout: 15_000 });

      // Confirm deletion via the API.
      const afterDelete = await page.request.get("/api/v1/keywords");
      const afterDeleteList = (await afterDelete.json()) as { term: string }[];
      expect(
        afterDeleteList.some((k) => k.term === term),
        "keyword absent from API after delete",
      ).toBe(false);
      createdId = null; // cleaned up successfully
    } finally {
      // Safety net: if anything above failed after creation, remove the row so reruns
      // stay clean. (Look it up fresh in case createdId was never captured.)
      if (createdId) {
        await page.request.delete(`/api/v1/keywords/${createdId}`);
      } else {
        const r = await page.request.get("/api/v1/keywords");
        const leftover = ((await r.json()) as { id: string; term: string }[]).find(
          (k) => k.term === term,
        );
        if (leftover) await page.request.delete(`/api/v1/keywords/${leftover.id}`);
      }
    }
  });

  test("toggling a keyword active/inactive reflects in the UI (self-resetting)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const term = `e2e toggle ${uniqueSuffix}`;

    // Create a dedicated keyword via the API so we never mutate the shared seeds.
    const createRes = await page.request.post("/api/v1/keywords", {
      data: { term, platforms: ["reddit"], match_type: "broad" },
    });
    expect(createRes.ok(), "create keyword (API) ok").toBeTruthy();
    const created = (await createRes.json()) as { id: string; is_active: boolean };
    expect(created.is_active, "new keyword starts active").toBe(true);
    const id = created.id;

    try {
      await page.goto(KEYWORDS_URL);
      const row = keywordRow(page, term);
      await expect(row, "keyword row visible").toBeVisible({ timeout: 15_000 });

      // Active rows render with bg-card (not the muted/opacity-60 inactive style).
      await expect(row).not.toHaveClass(/opacity-60/);

      // Click the toggle (first icon button) → updateKeyword({ is_active:false }).
      await toggleButton(row).click();

      // After the mutation + refetch the row picks up the inactive (opacity-60) style.
      await expect(
        keywordRow(page, term),
        "row becomes visually inactive",
      ).toHaveClass(/opacity-60/, { timeout: 15_000 });

      // Confirm the persisted flag via the API.
      const afterToggle = await page.request.get(`/api/v1/keywords/${id}`);
      expect(afterToggle.ok()).toBeTruthy();
      expect(((await afterToggle.json()) as { is_active: boolean }).is_active).toBe(
        false,
      );

      // Toggle back on → row returns to the active style.
      await toggleButton(keywordRow(page, term)).click();
      await expect(
        keywordRow(page, term),
        "row becomes active again",
      ).not.toHaveClass(/opacity-60/, { timeout: 15_000 });
    } finally {
      await page.request.delete(`/api/v1/keywords/${id}`);
    }
  });

  test("empty-state placeholder renders when the workspace has no keywords (non-destructive)", async ({
    page,
  }) => {
    // We must NOT delete the seeded keywords, so we exercise the empty-state branch by
    // intercepting the /keywords GET and returning an empty array for this page only.
    await page.route("**/api/v1/keywords", async (route) => {
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

    await page.goto(KEYWORDS_URL);

    await expect(
      page.getByText("No keywords yet. Add one above to start monitoring.", {
        exact: true,
      }),
      "empty-state placeholder visible",
    ).toBeVisible({ timeout: 15_000 });

    // The header count reads "0 keywords configured" (plural form, since 0 !== 1).
    await expect(
      page.getByText("0 keywords configured", { exact: true }),
      "header shows zero count",
    ).toBeVisible();

    await page.unroute("**/api/v1/keywords");
  });
});

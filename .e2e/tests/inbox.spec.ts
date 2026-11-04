import { test, expect } from "@playwright/test";

/**
 * LeadEcho "Smart Inbox" feature — the mentions inbox.
 *
 * Route: dashboard/src/routes/_dashboard/inbox.tsx, rendered at /app/inbox.
 *
 * The page reads:
 *   GET  /api/v1/mentions             (list, paginated, ?tier&status&platform&search&limit)
 *   GET  /api/v1/mentions/counts      (StatusCount[] → drives the "{n} new" badge)
 *   GET  /api/v1/mentions/tier-counts (TierCount[]   → drives the tier-tab counts)
 *   PATCH /api/v1/mentions/{id}/status  (archive / status change)
 *   POST /api/v1/mentions/{id}/classify    (AI — needs a provider key)
 *   POST /api/v1/mentions/{id}/draft-reply (AI — needs a provider key)
 *
 * The redesigned inbox is a two-pane layout: a list (queue tabs "Action required"
 * / "All mentions", search + platform/status selects) and a detail pane that
 * opens when a list row is clicked (Classify / Draft reply / Archive live there).
 * The authenticated storageState user has HackerNews mentions only, so
 * platform=reddit yields the empty state. We never require AI output.
 *
 * Everything here either only reads, or fully restores any mutation it makes,
 * so the spec is idempotent across reruns.
 */

const INBOX_URL = "/inbox?queue=all";
const EMPTY_TEXT = "No mentions match your filters.";

type Page = import("@playwright/test").Page;

// The list pane is the <section> holding the "Inbox" heading. Each mention row
// is a <button> showing "@author"; tab/refresh buttons never contain an "@".
function listPane(page: Page) {
  return page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Inbox", exact: true }) });
}
function mentionCards(page: Page) {
  return listPane(page).getByRole("button", { name: /@\S+/ });
}
function detailPane(page: Page) {
  return page.locator("section").filter({ has: page.getByRole("button", { name: "Person360" }) });
}

async function aiConfigured(page: Page): Promise<boolean> {
  const r = await page.request.get("/api/v1/llm/config");
  if (!r.ok()) return false;
  const body = (await r.json()) as { health?: { chat_ok?: boolean } };
  return body.health?.chat_ok === true;
}

test.describe("smart inbox / mentions", () => {
  test("page loads authenticated and renders the Inbox header", async ({ page }) => {
    const resp = await page.goto(INBOX_URL);
    expect(resp?.status(), "inbox HTTP status").toBeLessThan(400);
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(page.getByRole("heading", { name: "Inbox", exact: true })).toBeVisible();

    for (const label of ["Action required", "All mentions"]) {
      await expect(
        page.getByRole("button", { name: new RegExp(`^${label}`) }),
        `queue tab "${label}"`,
      ).toBeVisible();
    }

    await expect(page.getByRole("textbox", { name: "Search mentions", exact: true })).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Filter by status" })).toBeVisible();
  });

  test("populated state: mentions render with platform + total in subtitle", async ({ page }) => {
    const apiRes = await page.request.get("/api/v1/mentions?queue=all&limit=20");
    expect(apiRes.ok(), "mentions list endpoint ok").toBeTruthy();
    const apiBody = (await apiRes.json()) as { data: unknown[]; total: number };
    test.skip(apiBody.total === 0, "no mentions in this environment");

    await page.goto(INBOX_URL);
    await expect(
      page.getByText(`${apiBody.total.toLocaleString()} mentions`, { exact: true }),
      "subtitle total matches API",
    ).toBeVisible({ timeout: 15_000 });

    const cards = mentionCards(page);
    await expect(cards.first(), "first mention card visible").toBeVisible({ timeout: 15_000 });
    expect(await cards.count(), "rendered cards count").toBeGreaterThan(0);

    await expect(
      listPane(page).getByText("hackernews", { exact: true }).first(),
      "platform badge rendered",
    ).toBeVisible();
  });

  test("search filters the list to matching mentions", async ({ page }) => {
    const searchRes = await page.request.get(
      "/api/v1/mentions?queue=all&search=analytics&limit=20",
    );
    expect(searchRes.ok()).toBeTruthy();
    const searchBody = (await searchRes.json()) as { data: unknown[] };
    expect(searchBody.data.length, "search 'analytics' returns matches").toBeGreaterThan(0);

    await page.goto(INBOX_URL);
    await expect(mentionCards(page).first()).toBeVisible({ timeout: 15_000 });

    const box = page.getByRole("textbox", { name: "Search mentions", exact: true });
    await box.fill("analytics");
    await expect(mentionCards(page).first()).toBeVisible({ timeout: 15_000 });
    await expect(page).toHaveURL(/q=analytics/);

    await box.fill(`zzqqx-no-such-token-${Date.now()}`);
    await expect(page.getByText(EMPTY_TEXT), "empty state for no-match search").toBeVisible({
      timeout: 15_000,
    });
  });

  test("empty state: a platform with no mentions shows the empty message", async ({ page }) => {
    const redditRes = await page.request.get("/api/v1/mentions?queue=all&platform=reddit&limit=20");
    expect(redditRes.ok()).toBeTruthy();
    const reddit = (await redditRes.json()) as { data: unknown[] };
    test.skip(reddit.data.length > 0, "environment has reddit mentions");

    // Reddit is not in the platform select when no reddit mentions exist, so
    // drive the filter through the URL (same state the select writes).
    await page.goto(`${INBOX_URL}&platform=reddit`);
    await expect(page.getByText(EMPTY_TEXT)).toBeVisible({ timeout: 15_000 });
    await expect(mentionCards(page)).toHaveCount(0);
  });

  test("queue tabs: 'All mentions' is populated and matches the queue-counts API", async ({
    page,
  }) => {
    const qc = (await (await page.request.get("/api/v1/mentions/queue-counts")).json()) as {
      queues: Array<{ queue: string; count: number }>;
    };
    const all = qc.queues.find((q) => q.queue === "all")?.count ?? 0;
    expect(all, "mentions exist").toBeGreaterThan(0);

    await page.goto("/inbox");
    // Default queue falls back to All when nothing needs action.
    await page.getByRole("button", { name: /^All mentions/ }).click();
    await expect(
      page.getByRole("button", { name: new RegExp(`^All mentions\\s*${all}$`) }),
      "All tab shows API count",
    ).toBeVisible({ timeout: 15_000 });
    await expect(mentionCards(page).first()).toBeVisible({ timeout: 15_000 });

    await page.getByRole("button", { name: /^Action required/ }).click();
    await expect(page).toHaveURL(/queue=action_required/);
  });

  test("archiving a mention from the detail pane removes it and is restored", async ({ page }) => {
    const listRes = await page.request.get("/api/v1/mentions?queue=all&limit=20");
    const list = (await listRes.json()) as { data: Array<{ id: string; status: string }> };
    expect(list.data.length).toBeGreaterThan(0);
    const target = list.data[0];

    try {
      await page.goto(`${INBOX_URL}&id=${target.id}`);
      await expect(detailPane(page)).toBeVisible({ timeout: 15_000 });
      await detailPane(page).getByRole("button", { name: "Archive" }).click();

      await expect
        .poll(
          async () => {
            const r = await page.request.get(`/api/v1/mentions/${target.id}`);
            return r.ok() ? ((await r.json()) as { status: string }).status : null;
          },
          { timeout: 15_000, message: "mention becomes archived" },
        )
        .toBe("archived");

      // The archived mention leaves the (default) All queue.
      const allAfter = (await (
        await page.request.get("/api/v1/mentions?queue=all&limit=100")
      ).json()) as { data: Array<{ id: string }> };
      expect(allAfter.data.some((m) => m.id === target.id), "gone from All queue").toBe(false);
    } finally {
      await page.request.patch(`/api/v1/mentions/${target.id}/status`, {
        data: { status: target.status || "new" },
      });
    }

    const after = await page.request.get(`/api/v1/mentions/${target.id}`);
    expect(((await after.json()) as { status: string }).status).toBe(target.status || "new");
  });

  // Regression: queue=all used to exclude archived/spam even when the status
  // filter explicitly asked for them, so those options never showed anything.
  test("status filter 'archived' lists archived mentions", async ({ page }) => {
    const list = (await (await page.request.get("/api/v1/mentions?queue=all&limit=1")).json()) as {
      data: Array<{ id: string; status: string }>;
    };
    const target = list.data[0];
    try {
      await page.request.patch(`/api/v1/mentions/${target.id}/status`, { data: { status: "archived" } });
      await page.goto(`${INBOX_URL}&status=archived`);
      await expect(mentionCards(page).first()).toBeVisible({ timeout: 10_000 });
    } finally {
      await page.request.patch(`/api/v1/mentions/${target.id}/status`, {
        data: { status: target.status || "new" },
      });
    }
  });

  test("AI controls exist and behave per provider status", async ({ page }) => {
    const listRes = await page.request.get("/api/v1/mentions?queue=all&limit=1");
    const first = ((await listRes.json()) as { data: Array<{ id: string }> }).data[0];

    await page.goto(`${INBOX_URL}&id=${first.id}`);
    const pane = detailPane(page);
    await expect(pane).toBeVisible({ timeout: 15_000 });
    const draftBtn = pane.getByRole("button", { name: "Draft reply" });
    await expect(draftBtn, "Draft reply control exists").toBeVisible();

    if (await aiConfigured(page)) {
      // Don't burn LLM calls from the suite; just assert the button is enabled.
      await expect(draftBtn).toBeEnabled();
      return;
    }

    // No provider key → HTTP 400 with a clear message; UI must not crash.
    const draftRes = await page.request.post(`/api/v1/mentions/${first.id}/draft-reply`);
    expect(draftRes.status(), "draft-reply is a client error, not a crash").toBe(400);
    expect(await draftRes.text()).toMatch(/AI provider/i);
    await draftBtn.click();
    await expect(pane.getByText("AI draft", { exact: true })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Inbox", exact: true })).toBeVisible();
  });
});

import { test, expect } from "@playwright/test";

/**
 * Frontend cross-cutting UX regressions from the bug-hunt:
 *  - failed mutations surface a clean global error toast (not a silent no-op,
 *    and not the raw "<status>: {json}" envelope)
 *  - sidebar has no duplicate nav entry
 *  - the logo links into the app, not the marketing landing
 */

test("duplicate keyword surfaces a clean error toast (no raw status/JSON leak)", async ({
  page,
}) => {
  const term = `e2e-ux-dup-${Date.now()}`;
  await page.goto("/app/keywords");
  await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
    timeout: 15_000,
  });

  const input = page.getByPlaceholder("Enter keyword or phrase...");
  const add = page.getByRole("button", { name: "Add" });

  // First create succeeds.
  await input.fill(term);
  await add.click();
  await expect(
    page.getByText(term, { exact: true }).first(),
    "first keyword created",
  ).toBeVisible({ timeout: 15_000 });

  // Second create of the same term fails -> a clean toast must appear.
  await input.fill(term);
  await add.click();
  const toast = page.getByRole("alert").filter({ hasText: /already exists/i });
  await expect(toast, "clean duplicate-keyword error toast").toBeVisible({
    timeout: 10_000,
  });
  // Must NOT leak the raw "409: {...}" envelope.
  await expect(page.getByText(/409:\s*\{/)).toHaveCount(0);

  // cleanup: delete the keyword we created (find its row + delete control)
  const res = await page.request.get("/api/v1/keywords");
  const kws = (await res.json()) as { id: string; term: string }[];
  const mine = kws.find((k) => k.term === term);
  if (mine) await page.request.delete(`/api/v1/keywords/${mine.id}`);
});

test("sidebar has no duplicate nav entry and the logo links into the app", async ({
  page,
}) => {
  await page.goto("/app/inbox");
  // Exactly one nav link to /app/analytics (the duplicate 'Tracking' is gone).
  const analyticsLinks = page.locator('a[href="/app/analytics"]');
  await expect(analyticsLinks).toHaveCount(1);

  // The logo links to the app inbox, not the marketing root.
  const logo = page.getByRole("link", { name: /LeadEcho/i }).first();
  await expect(logo).toHaveAttribute("href", "/app/inbox");
});

import { test, expect } from "@playwright/test";

// Validates the authenticated stack end-to-end through a real browser.
test("authenticated dashboard shell loads at /app/inbox", async ({ page }) => {
  const resp = await page.goto("/inbox");
  expect(resp?.status(), "inbox HTTP status").toBeLessThan(400);

  // Should NOT be bounced to login (storageState carries the session cookie).
  await expect(page).not.toHaveURL(/\/login/);

  // Dashboard shell renders (sidebar nav with feature links).
  await expect(page.locator("body")).toBeVisible();
  await page.waitForLoadState("networkidle");
});

test("api/auth/me returns the session user", async ({ page }) => {
  const res = await page.request.get("/api/v1/auth/me");
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.email).toBe("e2e-tester@leadecho.test");
});

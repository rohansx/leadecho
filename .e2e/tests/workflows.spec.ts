import { test, expect } from "@playwright/test";

/**
 * LeadEcho "workflows" feature — the workflow engine page.
 *
 * Route: /opt/leadecho/dashboard/src/routes/_dashboard/workflows.tsx
 * URL:   /app/workflows
 *
 * This route is a STATIC, CLIENT-ONLY placeholder. The component (WorkflowsPage)
 * renders:
 *   - <Text as="h2">Workflows</Text>                       → a real <h2> heading
 *   - a <Card> whose <CardContent> holds the muted paragraph
 *       "Visual workflow builder with triggers, conditions, and actions
 *        coming soon."
 *
 * There are NO API calls, NO react-query, and NO mutations on this route — so
 * there is nothing to create/clean up. These tests assert the primary UI renders
 * without crashing, the route is reachable directly and via the sidebar nav, and
 * that no backend request is required to paint the page.
 *
 * The Playwright config supplies an authenticated storageState (onboarding-
 * complete user), so the spec starts logged in — just page.goto.
 */

const WORKFLOWS_URL = "/workflows";

// The exact placeholder copy rendered in workflows.tsx. The JSX wraps the line,
// so the live DOM normalizes the whitespace to a single space — match loosely
// on the stable leading phrase to stay robust to that wrapping.
const PLACEHOLDER_RE =
  /Visual workflow builder with triggers, conditions, and actions/i;

test.describe("workflows page", () => {
  test("loads authenticated and renders the Workflows heading", async ({
    page,
  }) => {
    const resp = await page.goto(WORKFLOWS_URL);
    expect(resp?.status(), "workflows HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login or onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);
    await expect(page).toHaveURL(/\/workflows/);

    // Primary UI: the <h2>Workflows</h2> heading (Text as="h2").
    await expect(
      page.getByRole("heading", { name: "Workflows", exact: true }),
    ).toBeVisible();
  });

  test("renders the 'coming soon' placeholder card without crashing", async ({
    page,
  }) => {
    await page.goto(WORKFLOWS_URL);

    // The placeholder copy inside the CardContent is the page's primary content.
    await expect(page.getByText(PLACEHOLDER_RE)).toBeVisible();
    await expect(page.getByText(/coming soon/i)).toBeVisible();

    // The dashboard shell is intact (sidebar present alongside the page body).
    await expect(page.locator("aside")).toBeVisible();
    await expect(page.locator("body")).toBeVisible();
  });

  test("page paints client-side with no /api/v1 request required", async ({
    page,
  }) => {
    // This route is purely static — it must not depend on any backend call to
    // render its heading + placeholder. Record any API calls it happens to make.
    const apiCalls: string[] = [];
    page.on("request", (req) => {
      const url = req.url();
      if (url.includes("/api/v1/")) apiCalls.push(url);
    });

    await page.goto(WORKFLOWS_URL);

    // Heading + placeholder are visible regardless of network.
    await expect(
      page.getByRole("heading", { name: "Workflows", exact: true }),
    ).toBeVisible();
    await expect(page.getByText(PLACEHOLDER_RE)).toBeVisible();

    // No workflows-specific endpoint exists; the component issues no data fetch.
    // (Auth/me or other shell calls would not target a /workflows resource.)
    const workflowApiCalls = apiCalls.filter((u) => /workflow/i.test(u));
    expect(
      workflowApiCalls,
      `no workflows API endpoint should be called (saw: ${workflowApiCalls.join(", ")})`,
    ).toHaveLength(0);
  });

  test("reachable from the sidebar 'Workflows' nav link", async ({ page }) => {
    // Start from a different dashboard route, then navigate via the sidebar so
    // we exercise the in-app router (not just a direct goto).
    await page.goto("/inbox");
    await expect(page).toHaveURL(/\/inbox/);

    const nav = page.locator("aside");
    const workflowsLink = nav.getByRole("link", { name: "Workflows" });
    await expect(workflowsLink, "sidebar Workflows link present").toBeVisible();

    await workflowsLink.click();

    // Client-side navigation lands on the workflows route and paints its UI.
    await expect(page).toHaveURL(/\/workflows/);
    await expect(
      page.getByRole("heading", { name: "Workflows", exact: true }),
    ).toBeVisible();
    await expect(page.getByText(PLACEHOLDER_RE)).toBeVisible();
  });

  test("is idempotent — repeated visits render identical stable UI", async ({
    page,
  }) => {
    // A static page should look the same on every visit (no flaky data state).
    for (let i = 0; i < 2; i++) {
      await page.goto(WORKFLOWS_URL);
      await expect(
        page.getByRole("heading", { name: "Workflows", exact: true }),
        `heading visible on visit #${i + 1}`,
      ).toBeVisible();
      await expect(
        page.getByText(PLACEHOLDER_RE),
        `placeholder visible on visit #${i + 1}`,
      ).toBeVisible();
    }

    // Exactly one Workflows heading is present (no duplicate render).
    await expect(
      page.getByRole("heading", { name: "Workflows", exact: true }),
    ).toHaveCount(1);
  });
});

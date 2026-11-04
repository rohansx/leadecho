import { test, expect, type Page } from "@playwright/test";

/**
 * LeadEcho "alerts" feature — webhook / notification channel config.
 *
 * Route: /_dashboard/alerts.tsx, rendered at /app/alerts.
 * Reads:
 *   GET  /api/v1/notifications/webhooks       -> WebhookConfig
 *        (the raw stored "webhooks" map merged with { resend_configured }; may be {}
 *         for a workspace that has never saved — the component then falls back to its
 *         own defaults via ?? / ||).
 * Mutates:
 *   PUT  /api/v1/notifications/webhooks        body {
 *          slack_url, discord_url, email_to, enabled,
 *          on_new_mention, on_high_intent, on_new_lead
 *        } -> the saved webhooks map (200). NOTE: resend_configured is server-derived
 *        and is NOT part of the PUT body.
 *   POST /api/v1/notifications/webhooks/test   body { channel, webhook_url } ->
 *          { status: "sent" } (200). Against a *fake* URL this may 200 (if the host is
 *          reachable and just returns an error code — http.Post still yields a response,
 *          err==nil) OR 502 (if the host can't be dialled). We therefore assert the UI
 *          degrades gracefully and never require success.
 *
 * UI shape (from alerts.tsx):
 *   - <h2> heading "Alerts" (Text as="h2" -> real <h2>).
 *   - "Enable notifications" master checkbox (first checkbox on the page).
 *   - Slack card:   <input placeholder="https://hooks.slack.com/services/..."> + "Test" button.
 *   - Discord card: <input placeholder="https://discord.com/api/webhooks/..."> + "Test" button.
 *   - Email card:   <input placeholder="alerts@yourcompany.com" type="email"> + "Test" button.
 *                   The email input + its Test button are DISABLED unless resend_configured.
 *                   A Badge shows either "Resend connected" or "Set RESEND_API_KEY in .env".
 *   - Event Triggers card: three checkboxes — "New mentions", "High-intent signals",
 *     "New leads created".
 *   - Footer: "Save Alert Settings" button. On test success an inline green
 *     "<Channel> test notification sent!" line shows for 3s; on save success (and when no
 *     test message is showing) a green "Settings saved." line shows.
 *
 * IMPORTANT — the webhook config is WORKSPACE-GLOBAL (a single settings row), not a
 * per-resource collection. So this spec cannot create a uniquely-suffixed throwaway row;
 * instead, every test that mutates the config first SNAPSHOTS the current config via the
 * API and RESTORES it in a finally block, so reruns and other specs are never disturbed.
 * The unique epoch-ms suffix is still used to make the saved Slack/Discord URLs unique so
 * we can positively assert *our* write persisted.
 */

const ALERTS_URL = "/alerts";

interface WebhookConfig {
  slack_url?: string;
  discord_url?: string;
  email_to?: string;
  enabled?: boolean;
  on_new_mention?: boolean;
  on_high_intent?: boolean;
  on_new_lead?: boolean;
  resend_configured?: boolean;
}

// Fields the PUT accepts (resend_configured is server-derived, never written back).
function toPutBody(c: WebhookConfig) {
  return {
    slack_url: c.slack_url ?? "",
    discord_url: c.discord_url ?? "",
    email_to: c.email_to ?? "",
    enabled: c.enabled ?? false,
    on_new_mention: c.on_new_mention ?? true,
    on_high_intent: c.on_high_intent ?? true,
    on_new_lead: c.on_new_lead ?? false,
  };
}

async function getConfig(page: Page): Promise<WebhookConfig> {
  const res = await page.request.get("/api/v1/notifications/webhooks");
  expect(res.ok(), "GET /notifications/webhooks ok").toBeTruthy();
  return (await res.json()) as WebhookConfig;
}

async function putConfig(page: Page, c: WebhookConfig) {
  const res = await page.request.put("/api/v1/notifications/webhooks", {
    data: toPutBody(c),
  });
  expect(res.ok(), "PUT /notifications/webhooks ok").toBeTruthy();
}

const slackInput = (page: Page) =>
  page.getByPlaceholder("https://hooks.slack.com/services/...");
const discordInput = (page: Page) =>
  page.getByPlaceholder("https://discord.com/api/webhooks/...");
const emailInput = (page: Page) =>
  page.getByPlaceholder("alerts@yourcompany.com");

test.describe("alerts / webhook notification config", () => {
  test("page loads authenticated and renders all channel cards", async ({
    page,
  }) => {
    const resp = await page.goto(ALERTS_URL);
    expect(resp?.status(), "alerts HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Alerts", exact: true }),
    ).toBeVisible();

    // All three channel cards render their inputs + the event-trigger checkboxes.
    await expect(slackInput(page)).toBeVisible();
    await expect(discordInput(page)).toBeVisible();
    await expect(emailInput(page)).toBeVisible();

    await expect(
      page.getByRole("heading", { name: "Slack", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Discord", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Event Triggers", exact: true }),
    ).toBeVisible();

    // The three event-trigger labels.
    await expect(page.getByText("New mentions", { exact: true })).toBeVisible();
    await expect(
      page.getByText("High-intent signals", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText("New leads created", { exact: true }),
    ).toBeVisible();

    // Save button present.
    await expect(
      page.getByRole("button", { name: "Save Alert Settings" }),
    ).toBeVisible();
  });

  test("form hydrates from the stored config (GET reflected into inputs)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const original = await getConfig(page);

    // Seed a known config via the API, then load the page and assert the inputs hydrate
    // from it. The component syncs API -> local state exactly once (the `!loaded` guard).
    const seededSlack = `https://hooks.slack.com/services/E2E/${uniqueSuffix}`;
    const seededDiscord = `https://discord.com/api/webhooks/${uniqueSuffix}/tok`;

    try {
      await putConfig(page, {
        ...original,
        slack_url: seededSlack,
        discord_url: seededDiscord,
        enabled: true,
        on_new_mention: true,
        on_high_intent: false,
        on_new_lead: true,
      });

      await page.goto(ALERTS_URL);

      // react-query resolves -> the controlled inputs pick up the stored URLs.
      await expect(slackInput(page)).toHaveValue(seededSlack, {
        timeout: 15_000,
      });
      await expect(discordInput(page)).toHaveValue(seededDiscord);

      // The master "Enable notifications" checkbox is the first checkbox on the page;
      // it reflects enabled=true. Event-trigger checkboxes reflect the stored flags.
      const checkboxes = page.locator('input[type="checkbox"]');
      // [0] master, [1] New mentions, [2] High-intent, [3] New leads.
      await expect(checkboxes.nth(0)).toBeChecked(); // enabled
      await expect(checkboxes.nth(1)).toBeChecked(); // on_new_mention = true
      await expect(checkboxes.nth(2)).not.toBeChecked(); // on_high_intent = false
      await expect(checkboxes.nth(3)).toBeChecked(); // on_new_lead = true
    } finally {
      await putConfig(page, original);
    }
  });

  test("save a Slack + Discord webhook URL via the UI and assert persistence", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const original = await getConfig(page);

    const newSlack = `https://hooks.slack.com/services/T00/B00/${uniqueSuffix}`;
    const newDiscord = `https://discord.com/api/webhooks/${uniqueSuffix}/abcdef`;

    try {
      await page.goto(ALERTS_URL);
      await expect(slackInput(page)).toBeVisible({ timeout: 15_000 });

      // Edit the Slack + Discord URLs and flip the master switch on.
      await slackInput(page).fill(newSlack);
      await discordInput(page).fill(newDiscord);

      const master = page.locator('input[type="checkbox"]').nth(0);
      if (!(await master.isChecked())) {
        await master.check();
      }

      await page
        .getByRole("button", { name: "Save Alert Settings" })
        .click();

      // Real behaviour: on save success the inline "Settings saved." line appears
      // (no test message is showing, so this branch renders).
      await expect(
        page.getByText("Settings saved.", { exact: true }),
        "save confirmation visible",
      ).toBeVisible({ timeout: 15_000 });

      // Persistence: the API now returns exactly what we typed.
      const after = await getConfig(page);
      expect(after.slack_url, "slack_url persisted").toBe(newSlack);
      expect(after.discord_url, "discord_url persisted").toBe(newDiscord);
      expect(after.enabled, "enabled persisted").toBe(true);

      // And a fresh page load re-hydrates the saved values into the inputs.
      await page.goto(ALERTS_URL);
      await expect(slackInput(page)).toHaveValue(newSlack, { timeout: 15_000 });
      await expect(discordInput(page)).toHaveValue(newDiscord);
    } finally {
      // Restore the workspace's pre-test config so reruns / other specs are unaffected.
      await putConfig(page, original);
    }
  });

  test("toggling an event-trigger checkbox persists through save", async ({
    page,
  }) => {
    const original = await getConfig(page);

    try {
      await page.goto(ALERTS_URL);
      await expect(slackInput(page)).toBeVisible({ timeout: 15_000 });

      // [2] = "High-intent signals". Capture its rendered state, flip it, save, assert.
      const highIntent = page.locator('input[type="checkbox"]').nth(2);
      const before = await highIntent.isChecked();
      if (before) {
        await highIntent.uncheck();
      } else {
        await highIntent.check();
      }
      await expect(highIntent).toBeChecked({ checked: !before });

      await page.getByRole("button", { name: "Save Alert Settings" }).click();
      await expect(
        page.getByText("Settings saved.", { exact: true }),
      ).toBeVisible({ timeout: 15_000 });

      const after = await getConfig(page);
      expect(after.on_high_intent, "on_high_intent flipped & persisted").toBe(
        !before,
      );
    } finally {
      await putConfig(page, original);
    }
  });

  test("Test button against a fake Slack URL is handled gracefully (no crash, no required success)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const original = await getConfig(page);
    // A syntactically valid but non-functional webhook URL. The backend either reaches
    // the host and gets an error code (-> 200 "sent", UI shows the green line) or fails
    // to dial it (-> 502, the mutation rejects and NO line shows). Both are acceptable;
    // we only require the page stays alive and interactive.
    const fakeSlack = `https://hooks.slack.com/services/FAKE/${uniqueSuffix}/doesnotexist`;

    try {
      await page.goto(ALERTS_URL);
      await expect(slackInput(page)).toBeVisible({ timeout: 15_000 });

      await slackInput(page).fill(fakeSlack);

      // The Slack "Test" button is the first "Test"-labelled button (Slack card is first).
      const slackTest = page
        .getByRole("button", { name: "Test" })
        .first();
      await expect(slackTest).toBeEnabled();
      await slackTest.click();

      // Graceful handling = the button returns to an enabled, clickable state (the
      // mutation settled, success or failure) and the page did not blow up.
      await expect(slackTest).toBeEnabled({ timeout: 15_000 });

      // The heading is still rendered (no unhandled error tore down the route) and the
      // URL we typed is still in the input (no crash/reset).
      await expect(
        page.getByRole("heading", { name: "Alerts", exact: true }),
      ).toBeVisible();
      await expect(slackInput(page)).toHaveValue(fakeSlack);

      // If a success line did appear, it must be the Slack one (never an uncaught error
      // toast). We do NOT assert it appears — failure is a valid outcome here.
      const successLine = page.getByText("Slack test notification sent!", {
        exact: true,
      });
      const count = await successLine.count();
      expect(count === 0 || count === 1, "at most one slack success line").toBe(
        true,
      );
    } finally {
      await putConfig(page, original);
    }
  });

  test("Test button is disabled until a webhook URL is entered", async ({
    page,
  }) => {
    const original = await getConfig(page);

    try {
      // Force an empty Slack URL so the Test button starts disabled regardless of
      // whatever the workspace currently has stored. Restore afterwards.
      await putConfig(page, { ...original, slack_url: "" });

      await page.goto(ALERTS_URL);
      await expect(slackInput(page)).toHaveValue("", { timeout: 15_000 });

      const slackTest = page.getByRole("button", { name: "Test" }).first();
      // disabled={!slackUrl || isPending} -> empty URL means disabled.
      await expect(slackTest).toBeDisabled();

      // Typing a URL enables it.
      await slackInput(page).fill("https://hooks.slack.com/services/x/y/z");
      await expect(slackTest).toBeEnabled();
    } finally {
      await putConfig(page, original);
    }
  });

  test("email channel reflects the server-side Resend configuration", async ({
    page,
  }) => {
    const cfg = await getConfig(page);

    await page.goto(ALERTS_URL);
    await expect(emailInput(page)).toBeVisible({ timeout: 15_000 });

    // resend_configured is server-derived (RESEND_API_KEY present?). The input + the
    // surrounding affordances must match it. We assert the branch that the server reports
    // rather than assuming the test env has Resend configured.
    if (cfg.resend_configured) {
      await expect(
        page.getByText("Resend connected", { exact: true }),
      ).toBeVisible();
      await expect(emailInput(page)).toBeEnabled();
    } else {
      await expect(
        page.getByText("Set RESEND_API_KEY in .env", { exact: true }),
      ).toBeVisible();
      // Input + its Test button are disabled when Resend is not configured.
      await expect(emailInput(page)).toBeDisabled();
      await expect(
        page.getByText(
          "Add your Resend API key to .env to enable email alerts. Free tier: 3,000 emails/month.",
          { exact: true },
        ),
      ).toBeVisible();
    }
  });

  test("empty/never-saved config renders sensible defaults (mocked GET)", async ({
    page,
  }) => {
    // Exercise the never-configured branch without disturbing the real stored config:
    // intercept the GET and return an empty object (what the backend returns when the
    // workspace has no "webhooks" key). The component then falls back to its own
    // defaults: empty URL fields, master OFF, new-mention ON, high-intent ON, new-lead OFF.
    await page.route("**/api/v1/notifications/webhooks", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ resend_configured: false }),
        });
        return;
      }
      await route.continue();
    });

    await page.goto(ALERTS_URL);
    await expect(slackInput(page)).toBeVisible({ timeout: 15_000 });

    // URL fields empty.
    await expect(slackInput(page)).toHaveValue("");
    await expect(discordInput(page)).toHaveValue("");
    await expect(emailInput(page)).toHaveValue("");

    const checkboxes = page.locator('input[type="checkbox"]');
    await expect(checkboxes.nth(0)).not.toBeChecked(); // enabled default false
    await expect(checkboxes.nth(1)).toBeChecked(); // on_new_mention default true
    await expect(checkboxes.nth(2)).toBeChecked(); // on_high_intent default true
    await expect(checkboxes.nth(3)).not.toBeChecked(); // on_new_lead default false

    await page.unroute("**/api/v1/notifications/webhooks");
  });
});

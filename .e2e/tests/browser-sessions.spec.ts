import { test, expect, type Page, type Locator } from "@playwright/test";

/**
 * LeadEcho "browser-sessions" feature — per-platform session (cookie) management.
 *
 * Route: /_dashboard/browser-sessions.tsx, rendered at /app/browser-sessions.
 *
 * Reads:
 *   GET  /api/v1/settings/sessions             -> PlatformSession[] (bare array, always
 *                                                  3 rows: reddit, twitter, linkedin)
 * Mutates:
 *   PUT    /api/v1/settings/sessions/{platform}  body { session_cookie, username? }
 *                                                  -> PlatformSession (is_configured:true)
 *   DELETE /api/v1/settings/sessions/{platform}  -> PlatformSession (is_configured:false)
 *   POST   /api/v1/settings/sessions/{platform}/test -> { pinchtab_online, message }
 *
 * PlatformSession = { platform: "reddit"|"twitter"|"linkedin", username: string|null,
 *                     is_configured: boolean, is_pinchtab_online: boolean }
 *
 * UI shape (from browser-sessions.tsx + card.tsx + badge.tsx):
 *   - Page heading <h2> "Browser Sessions" (Text as="h2") + an intro paragraph.
 *   - One <Card> per platform. The platform label is a CardTitle = <h3>:
 *       reddit -> "Reddit", twitter -> "Twitter / X", linkedin -> "LinkedIn".
 *   - Each card shows a status Badge: configured -> "Connected as <username>" (or
 *     "Connected" when username is null) with a CheckCircle2 icon; unconfigured ->
 *     "Not configured" with a Circle icon.
 *   - Each card shows a Pinchtab status line in the CardDescription: online ->
 *     "Pinchtab online"; offline -> "Pinchtab offline — set PINCHTAB_TOKEN to enable".
 *   - Each card has TWO inputs:
 *       * cookie input  -> type="password", placeholder is platform-specific
 *                          (e.g. reddit: 'Paste your reddit_session cookie value...').
 *                          NOTE password inputs have NO implicit ARIA role, so we
 *                          select these by placeholder, never getByRole("textbox").
 *       * username input -> placeholder is platform-specific (e.g. reddit: 'u/yourname').
 *   - A hint line + an "Cookies are encrypted at rest using AES-256-GCM." line.
 *   - Buttons: "Save" (disabled until the cookie input is non-empty), "Remove"
 *     (only rendered when is_configured), and "Test" (always present). After a
 *     successful Test the returned message renders as inline text.
 *
 * LIVE ENVIRONMENT REALITY (probed against the running backend):
 *   - The seeded workspace starts with ALL THREE platforms is_configured:false.
 *   - There is NO Pinchtab sidecar (PINCHTAB_TOKEN unset), so is_pinchtab_online is
 *     false everywhere and POST .../test returns:
 *         { pinchtab_online:false,
 *           message:"Pinchtab not configured — set PINCHTAB_TOKEN to enable" }
 *   - Saving a cookie STILL succeeds (cookie is encrypted + stored, is_configured
 *     flips true) even though no live session can actually be used — exactly the
 *     "assert UI presence + graceful handling, do not require a live session" goal.
 *
 * IDEMPOTENCY / SELF-CLEANING: the only mutating test operates on the "linkedin"
 * platform with a unique epoch-ms username suffix and ALWAYS deletes the linkedin
 * session in a finally block (DELETE is idempotent server-side), restoring the
 * seeded all-unconfigured baseline. The empty/populated render branches that would
 * otherwise need real saved state are exercised via per-page route interception so
 * the live backend is never mutated by them.
 */

const SESSIONS_URL = "/browser-sessions";

const PLATFORMS = ["reddit", "twitter", "linkedin"] as const;
type PlatformName = (typeof PLATFORMS)[number];

const PLATFORM_LABEL: Record<PlatformName, string> = {
  reddit: "Reddit",
  twitter: "Twitter / X",
  linkedin: "LinkedIn",
};

const COOKIE_PLACEHOLDER: Record<PlatformName, string> = {
  reddit: "Paste your reddit_session cookie value...",
  twitter: "Paste your auth_token cookie value...",
  linkedin: "Paste your li_at cookie value...",
};

const USERNAME_PLACEHOLDER: Record<PlatformName, string> = {
  reddit: "u/yourname",
  twitter: "@yourhandle",
  linkedin: "Your LinkedIn name",
};

interface PlatformSession {
  platform: PlatformName;
  username: string | null;
  is_configured: boolean;
  is_pinchtab_online: boolean;
}

// Locate a platform's <Card> by its CardTitle (<h3>) label; climb to the nearest
// Card root (div.glass-card). Each card is self-contained so this scopes every
// per-platform assertion (inputs, badges, buttons) to one platform.
function platformCard(page: Page, platform: PlatformName): Locator {
  return page
    .getByRole("heading", { name: PLATFORM_LABEL[platform], exact: true })
    .locator("xpath=ancestor::div[contains(@class,'glass-card')][1]");
}

test.describe("browser-sessions / per-platform session management", () => {
  test("page loads authenticated and renders the Browser Sessions heading + intro", async ({
    page,
  }) => {
    const resp = await page.goto(SESSIONS_URL);
    expect(resp?.status(), "browser-sessions HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Browser Sessions", exact: true }),
    ).toBeVisible();

    // Intro paragraph copy (mentions Reddit + Twitter + Pinchtab).
    await expect(
      page.getByText(/authenticated crawling via Pinchtab/i),
      "intro paragraph visible",
    ).toBeVisible();

    // Loading line clears once react-query resolves.
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });
  });

  test("renders one card per supported platform with its cookie + username inputs", async ({
    page,
  }) => {
    await page.goto(SESSIONS_URL);

    for (const platform of PLATFORMS) {
      const card = platformCard(page, platform);
      await expect(
        card,
        `${platform} card visible`,
      ).toBeVisible({ timeout: 15_000 });

      // Platform label heading.
      await expect(
        card.getByRole("heading", { name: PLATFORM_LABEL[platform], exact: true }),
        `${platform} title`,
      ).toBeVisible();

      // "Session Cookie" + "Username (optional)" field labels.
      await expect(
        card.getByText("Session Cookie", { exact: true }),
        `${platform} cookie label`,
      ).toBeVisible();
      await expect(
        card.getByText("Username (optional)", { exact: true }),
        `${platform} username label`,
      ).toBeVisible();

      // Cookie input (password) — selected by placeholder, NOT by textbox role.
      const cookieInput = card.getByPlaceholder(COOKIE_PLACEHOLDER[platform]);
      await expect(cookieInput, `${platform} cookie input`).toBeVisible();
      await expect(
        cookieInput,
        `${platform} cookie input is type=password`,
      ).toHaveAttribute("type", "password");

      // Username input — selected by placeholder.
      await expect(
        card.getByPlaceholder(USERNAME_PLACEHOLDER[platform]),
        `${platform} username input`,
      ).toBeVisible();

      // Encryption-at-rest reassurance copy.
      await expect(
        card.getByText("Cookies are encrypted at rest using AES-256-GCM.", {
          exact: true,
        }),
        `${platform} encryption note`,
      ).toBeVisible();

      // Save + Test buttons always present (Save starts disabled — empty cookie).
      await expect(
        card.getByRole("button", { name: "Save", exact: true }),
        `${platform} Save button`,
      ).toBeVisible();
      await expect(
        card.getByRole("button", { name: "Save", exact: true }),
        `${platform} Save disabled while cookie empty`,
      ).toBeDisabled();
      await expect(
        card.getByRole("button", { name: "Test", exact: true }),
        `${platform} Test button`,
      ).toBeVisible();
    }
  });

  test("session status + pinchtab badges mirror the /settings/sessions API", async ({
    page,
  }) => {
    // The API returns a bare array of exactly the three supported platforms.
    const res = await page.request.get("/api/v1/settings/sessions");
    expect(res.ok(), "/settings/sessions ok").toBeTruthy();
    const sessions = (await res.json()) as PlatformSession[];
    expect(Array.isArray(sessions), "sessions payload is an array").toBeTruthy();

    const byPlatform = new Map(sessions.map((s) => [s.platform, s]));
    for (const platform of PLATFORMS) {
      expect(
        byPlatform.has(platform),
        `API includes "${platform}" row`,
      ).toBeTruthy();
    }

    await page.goto(SESSIONS_URL);

    for (const platform of PLATFORMS) {
      const s = byPlatform.get(platform)!;
      const card = platformCard(page, platform);
      await expect(card).toBeVisible({ timeout: 15_000 });

      // Configured-state badge.
      if (s.is_configured) {
        const expected = s.username
          ? `Connected as ${s.username}`
          : "Connected";
        await expect(
          card.getByText(expected, { exact: true }),
          `${platform} connected badge`,
        ).toBeVisible();
        // Remove button only renders for configured platforms.
        await expect(
          card.getByRole("button", { name: "Remove", exact: true }),
          `${platform} Remove button (configured)`,
        ).toBeVisible();
      } else {
        await expect(
          card.getByText("Not configured", { exact: true }),
          `${platform} not-configured badge`,
        ).toBeVisible();
        // No Remove button when unconfigured.
        await expect(
          card.getByRole("button", { name: "Remove", exact: true }),
          `${platform} Remove absent (unconfigured)`,
        ).toHaveCount(0);
      }

      // Pinchtab status line reflects is_pinchtab_online.
      if (s.is_pinchtab_online) {
        await expect(
          card.getByText("Pinchtab online", { exact: true }),
          `${platform} pinchtab online`,
        ).toBeVisible();
      } else {
        await expect(
          card.getByText("Pinchtab offline — set PINCHTAB_TOKEN to enable", {
            exact: true,
          }),
          `${platform} pinchtab offline`,
        ).toBeVisible();
      }
    }
  });

  test('Test button surfaces graceful "Pinchtab not configured" message (no live sidecar)', async ({
    page,
  }) => {
    // Sanity-check the live contract first: with no PINCHTAB_TOKEN the endpoint
    // returns a 200 with pinchtab_online:false and the not-configured message.
    const apiRes = await page.request.post(
      "/api/v1/settings/sessions/reddit/test",
    );
    expect(apiRes.ok(), "test endpoint returns 200").toBeTruthy();
    const body = (await apiRes.json()) as {
      pinchtab_online: boolean;
      message: string;
    };
    expect(
      body.pinchtab_online,
      "no sidecar → pinchtab_online false",
    ).toBe(false);
    expect(typeof body.message, "test message is a string").toBe("string");
    expect(body.message.length, "test message is non-empty").toBeGreaterThan(0);

    await page.goto(SESSIONS_URL);
    const card = platformCard(page, "reddit");
    await expect(card).toBeVisible({ timeout: 15_000 });

    // Click Test → testMutation resolves → the message string renders inline.
    await card.getByRole("button", { name: "Test", exact: true }).click();

    // The exact message returned by the backend appears in the card. Graceful
    // handling: the click never throws and never requires a live session.
    await expect(
      // UI prefixes a status glyph ("✗ " / "✓ "), so match as a substring.
      card.getByText(body.message),
      "test result message rendered inline",
    ).toBeVisible({ timeout: 15_000 });
  });

  test("Save is gated on a non-empty cookie (button enables only after typing)", async ({
    page,
  }) => {
    await page.goto(SESSIONS_URL);
    const card = platformCard(page, "twitter");
    await expect(card).toBeVisible({ timeout: 15_000 });

    const save = card.getByRole("button", { name: "Save", exact: true });
    const cookieInput = card.getByPlaceholder(COOKIE_PLACEHOLDER.twitter);

    // Empty cookie → disabled.
    await expect(save, "Save disabled when cookie empty").toBeDisabled();

    // Whitespace-only must NOT enable (disabled={!cookie.trim()}).
    await cookieInput.fill("   ");
    await expect(
      save,
      "Save stays disabled for whitespace-only cookie",
    ).toBeDisabled();

    // Real value → enabled.
    await cookieInput.fill("some-cookie-value");
    await expect(save, "Save enabled with a non-empty cookie").toBeEnabled();

    // Clear it again → disabled returns. (No mutation performed; nothing to clean.)
    await cookieInput.fill("");
    await expect(save, "Save disabled again after clearing").toBeDisabled();
  });

  test("save a LinkedIn session via the UI → Connected badge + Remove appears; then remove it (self-cleaning)", async ({
    page,
  }) => {
    const platform: PlatformName = "linkedin";
    const username = `e2e-li-${Date.now()}`;
    let saved = false;

    // Precondition: linkedin must start unconfigured (seeded baseline). If a prior
    // crashed run left it configured, reset it so this test is deterministic.
    const pre = await page.request.get("/api/v1/settings/sessions");
    const preState = ((await pre.json()) as PlatformSession[]).find(
      (s) => s.platform === platform,
    );
    if (preState?.is_configured) {
      await page.request.delete(`/api/v1/settings/sessions/${platform}`);
    }

    try {
      await page.goto(SESSIONS_URL);
      const card = platformCard(page, platform);
      await expect(card).toBeVisible({ timeout: 15_000 });

      // Starts unconfigured: "Not configured" badge, no Remove button.
      await expect(
        card.getByText("Not configured", { exact: true }),
        "linkedin starts not-configured",
      ).toBeVisible();
      await expect(
        card.getByRole("button", { name: "Remove", exact: true }),
      ).toHaveCount(0);

      // Fill cookie + username and Save (saveMutation → PUT, then invalidate).
      await card.getByPlaceholder(COOKIE_PLACEHOLDER[platform]).fill(
        "fake-li_at-cookie-value-for-e2e",
      );
      await card.getByPlaceholder(USERNAME_PLACEHOLDER[platform]).fill(username);

      const save = card.getByRole("button", { name: "Save", exact: true });
      await expect(save).toBeEnabled();
      await save.click();
      saved = true;

      // Real behavior after the mutation + list refetch: the status badge flips to
      // "Connected as <username>" and the Remove button appears. (Graceful even
      // though no live Pinchtab session exists.)
      await expect(
        card.getByText(`Connected as ${username}`, { exact: true }),
        "Connected badge with username after save",
      ).toBeVisible({ timeout: 15_000 });
      await expect(
        card.getByRole("button", { name: "Remove", exact: true }),
        "Remove button appears once configured",
      ).toBeVisible();

      // onSuccess clears the cookie + username inputs.
      await expect(
        card.getByPlaceholder(COOKIE_PLACEHOLDER[platform]),
        "cookie input cleared on save success",
      ).toHaveValue("");
      await expect(
        card.getByPlaceholder(USERNAME_PLACEHOLDER[platform]),
        "username input cleared on save success",
      ).toHaveValue("");

      // Pinchtab is still offline (no sidecar) — assert graceful offline line.
      await expect(
        card.getByText("Pinchtab offline — set PINCHTAB_TOKEN to enable", {
          exact: true,
        }),
        "pinchtab still reported offline after save",
      ).toBeVisible();

      // Confirm persistence via the API.
      const afterSave = await page.request.get("/api/v1/settings/sessions");
      const savedState = ((await afterSave.json()) as PlatformSession[]).find(
        (s) => s.platform === platform,
      );
      expect(savedState?.is_configured, "linkedin configured in API").toBe(true);
      expect(savedState?.username, "username persisted in API").toBe(username);

      // ── Now REMOVE via the UI (the behavior under test) ──
      await card.getByRole("button", { name: "Remove", exact: true }).click();

      // Badge reverts to "Not configured" and Remove disappears after refetch.
      await expect(
        card.getByText("Not configured", { exact: true }),
        "reverts to not-configured after remove",
      ).toBeVisible({ timeout: 15_000 });
      await expect(
        card.getByRole("button", { name: "Remove", exact: true }),
        "Remove button gone after remove",
      ).toHaveCount(0);

      // Confirm deletion via the API.
      const afterDelete = await page.request.get("/api/v1/settings/sessions");
      const deletedState = ((await afterDelete.json()) as PlatformSession[]).find(
        (s) => s.platform === platform,
      );
      expect(
        deletedState?.is_configured,
        "linkedin unconfigured in API after remove",
      ).toBe(false);
      saved = false; // cleaned up via the UI path
    } finally {
      // Safety net: DELETE is idempotent server-side, so always restore baseline.
      if (saved) {
        await page.request.delete(`/api/v1/settings/sessions/${platform}`);
      } else {
        // Even on the happy path, guarantee the seeded baseline (no-op if absent).
        const r = await page.request.get("/api/v1/settings/sessions");
        const leftover = ((await r.json()) as PlatformSession[]).find(
          (s) => s.platform === platform && s.is_configured,
        );
        if (leftover) {
          await page.request.delete(`/api/v1/settings/sessions/${platform}`);
        }
      }
    }
  });

  test("configured + pinchtab-online rendering (mocked list, no live mutation)", async ({
    page,
  }) => {
    // Exercise the "configured" and "Pinchtab online" UI branches WITHOUT mutating
    // the live backend, by intercepting the list GET for this page only.
    const mocked: PlatformSession[] = [
      {
        platform: "reddit",
        username: "mock_redditor",
        is_configured: true,
        is_pinchtab_online: true,
      },
      {
        platform: "twitter",
        username: null,
        is_configured: true,
        is_pinchtab_online: true,
      },
      {
        platform: "linkedin",
        username: null,
        is_configured: false,
        is_pinchtab_online: true,
      },
    ];

    await page.route("**/api/v1/settings/sessions", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mocked),
        });
        return;
      }
      await route.continue();
    });

    await page.goto(SESSIONS_URL);

    // reddit: configured WITH username → "Connected as mock_redditor" + Remove.
    const reddit = platformCard(page, "reddit");
    await expect(reddit).toBeVisible({ timeout: 15_000 });
    await expect(
      reddit.getByText("Connected as mock_redditor", { exact: true }),
      "reddit connected-with-username badge",
    ).toBeVisible();
    await expect(
      reddit.getByRole("button", { name: "Remove", exact: true }),
    ).toBeVisible();
    await expect(
      reddit.getByText("Pinchtab online", { exact: true }),
      "reddit pinchtab-online line",
    ).toBeVisible();

    // twitter: configured WITHOUT username → bare "Connected" badge.
    const twitter = platformCard(page, "twitter");
    await expect(
      twitter.getByText("Connected", { exact: true }),
      "twitter connected (no username) badge",
    ).toBeVisible();
    await expect(
      twitter.getByRole("button", { name: "Remove", exact: true }),
    ).toBeVisible();

    // linkedin: unconfigured even though pinchtab is online → "Not configured",
    // online line present, no Remove.
    const linkedin = platformCard(page, "linkedin");
    await expect(
      linkedin.getByText("Not configured", { exact: true }),
      "linkedin not-configured badge",
    ).toBeVisible();
    await expect(
      linkedin.getByText("Pinchtab online", { exact: true }),
    ).toBeVisible();
    await expect(
      linkedin.getByRole("button", { name: "Remove", exact: true }),
    ).toHaveCount(0);

    await page.unroute("**/api/v1/settings/sessions");
  });
});

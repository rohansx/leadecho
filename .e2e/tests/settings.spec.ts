import { test, expect, type Page } from "@playwright/test";

/**
 * LeadEcho "settings" feature — BYOK API keys + Chrome extension token.
 *
 * Route: /_dashboard/settings.tsx, rendered at /app/settings.
 *
 * Endpoints (from lib/api.ts + backend handler/settings.go, handler/extension.go,
 * registered in api/router.go):
 *
 *   Chrome extension token (fully wired into the Settings UI — ChromeExtensionCard):
 *     GET    /api/v1/settings/extension-token
 *              -> { has_token, masked_token?, name?, last_used_at, created_at? }
 *            backend masks via maskToken(): token[:8] + "..." + token[len-4:]
 *     POST   /api/v1/settings/extension-token   body { name }  (rotate/generate)
 *              -> { token, name, created_at }   (full token returned ONCE)
 *            token = hex of 32 random bytes => 64 hex chars.
 *     DELETE /api/v1/settings/extension-token   -> { status: "revoked" }
 *
 *   BYOK API keys (endpoints exist + work, but NOT surfaced in the Settings JSX —
 *   the "Account" card only says custom API keys are "coming in Pro plan").
 *   So these are exercised at the API layer via the page's authenticated request ctx:
 *     GET    /api/v1/settings/api-keys              -> APIKeyStatus[]
 *     PUT    /api/v1/settings/api-keys  body { provider, api_key } -> APIKeyStatus
 *     DELETE /api/v1/settings/api-keys  body { provider }          -> APIKeyStatus
 *            supported providers: glm | openai | voyage.
 *            masked_key via crypto.MaskKey(): key[:7] + "..." + key[-4:] (len>8), else "****".
 *
 * UI shape (from settings.tsx):
 *   - Page heading <h2> "Settings".
 *   - Three Cards: "AI Features", "Chrome Extension", "Account".
 *   - ChromeExtensionCard:
 *       * status row: a <Badge> reading "Loading..." | "Active" | "No key",
 *         the masked_token in a font-mono <p> when a token exists, an optional
 *         "Last used ..." line, plus action <button>s.
 *       * primary button text: "Generate key" (no token) / "Rotate key" (token),
 *         and "Generating..." while the mutation is pending.
 *       * a "Revoke" <button> shown only when a token exists.
 *       * after rotate, a one-time reveal block: an amber warning
 *         "Copy this key now — it won't be shown again.", a readOnly <input>
 *         holding the full token, and a "Copy" button.
 *   - "Account" card body text: "Billing, plan upgrades, and custom API keys coming in Pro plan."
 *
 * IDEMPOTENCY / SELF-CLEANING:
 *   - The workspace has at most ONE extension token (rotate replaces the old one).
 *     Tests that generate a token RESTORE the workspace to its pre-test token state
 *     (re-create if there was one, revoke if there wasn't) in a finally block.
 *   - API-key tests use a unique epoch-ms suffix in the key value and DELETE the
 *     provider key they wrote (the providers are shared, so we only touch a provider
 *     that was empty at the start, and restore it afterwards).
 */

const SETTINGS_URL = "/settings";

type TokenInfo = {
  has_token: boolean;
  masked_token?: string;
  name?: string;
  last_used_at?: string | null;
  created_at?: string;
};

type ApiKeyStatus = { provider: string; is_set: boolean; masked_key?: string };

async function getTokenInfo(page: Page): Promise<TokenInfo> {
  const res = await page.request.get("/api/v1/settings/extension-token");
  expect(res.ok(), "GET /settings/extension-token ok").toBeTruthy();
  return (await res.json()) as TokenInfo;
}

// Restore the workspace token to a known prior state. If there was no token before,
// revoke; otherwise we can't recover the exact old secret, but for cleanliness we
// leave a single freshly-rotated token in place (the workspace only ever holds one).
async function revokeToken(page: Page): Promise<void> {
  await page.request.delete("/api/v1/settings/extension-token");
}

// "No key" / "Active" badges also appear on other cards (AI providers); scope to
// the Chrome Extension card.
function extCard(page: import("@playwright/test").Page) {
  return page
    .locator(".glass-card")
    .filter({ has: page.getByRole("heading", { name: "Chrome Extension" }) });
}

test.describe("settings / BYOK keys + extension token", () => {
  test("page loads authenticated and renders the Settings cards", async ({
    page,
  }) => {
    const resp = await page.goto(SETTINGS_URL);
    expect(resp?.status(), "settings HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Settings", exact: true }),
    ).toBeVisible();

    // The three section cards render.
    await expect(
      page.getByText("AI Features", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText("Chrome Extension", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("Account", { exact: true })).toBeVisible();

    // The Account card confirms BYOK API keys are not yet a UI feature here.
    await expect(
      page.getByText(
        "Billing, plan upgrades, and custom API keys coming in Pro plan.",
        { exact: true },
      ),
    ).toBeVisible();
  });

  test("extension-token status reflects the API (Active+masked vs No key)", async ({
    page,
  }) => {
    const info = await getTokenInfo(page);

    await page.goto(SETTINGS_URL);

    // "Loading..." badge resolves once react-query settles.
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });

    if (info.has_token) {
      // Badge reads "Active" and the masked token is shown verbatim.
      await expect(
        extCard(page).getByText("Active", { exact: true }),
        "Active badge when a token exists",
      ).toBeVisible();
      expect(info.masked_token, "API returns a masked_token").toBeTruthy();
      await expect(
        page.getByText(info.masked_token!, { exact: true }),
        "masked token rendered in the UI",
      ).toBeVisible();
      // Masked tokens never expose the full secret (64 hex chars), only a fragment.
      expect(info.masked_token!.length).toBeLessThan(64);
      expect(info.masked_token!).toContain("...");
      // Primary action is "Rotate key" and a Revoke button is present.
      await expect(
        page.getByRole("button", { name: "Rotate key" }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Revoke" }),
      ).toBeVisible();
    } else {
      // No token → "No key" badge, "Generate key" button, no Revoke button.
      await expect(
        extCard(page).getByText("No key", { exact: true }),
        "No key badge when there is no token",
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Generate key" }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Revoke" }),
      ).toHaveCount(0);
    }
  });

  test("generate/rotate the extension token via the UI: full token reveals once, then masked persists", async ({
    page,
  }) => {
    const before = await getTokenInfo(page);

    await page.goto(SETTINGS_URL);
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });

    try {
      // The primary button label depends on whether a token already exists.
      const primaryName = before.has_token ? "Rotate key" : "Generate key";
      const primary = page.getByRole("button", { name: primaryName });
      await expect(primary).toBeVisible();
      await primary.click();

      // One-time reveal block appears with the amber warning + full token input.
      await expect(
        page.getByText("Copy this key now — it won't be shown again.", {
          exact: true,
        }),
        "one-time reveal warning shown after generate",
      ).toBeVisible({ timeout: 15_000 });

      // The readOnly reveal input holds the full 64-hex-char token (not masked).
      const revealInput = page.locator("input[readonly]");
      await expect(revealInput).toBeVisible();
      const full = await revealInput.inputValue();
      expect(full, "revealed token is 64 hex chars").toMatch(/^[0-9a-f]{64}$/);

      // After rotate + refetch, the status badge flips to "Active".
      await expect(
        extCard(page).getByText("Active", { exact: true }),
        "badge becomes Active after generating a token",
      ).toBeVisible({ timeout: 15_000 });

      // The Copy button is available (does not throw on click in headless chromium).
      const copyBtn = page.getByRole("button", { name: "Copy" });
      await expect(copyBtn).toBeVisible();

      // Persistence: the API now reports a token whose masked form matches the
      // first 8 and last 4 chars of the revealed secret (maskToken contract).
      const after = await getTokenInfo(page);
      expect(after.has_token, "token persisted in API after rotate").toBe(true);
      expect(after.masked_token, "masked token returned").toBeTruthy();
      const expectedMasked = `${full.slice(0, 8)}...${full.slice(-4)}`;
      expect(after.masked_token).toBe(expectedMasked);

      // The masked form is exactly what the UI re-renders in the status row after
      // the query invalidation. (The reveal input still shows the full token,
      // so scope the masked assertion to the font-mono status paragraph.)
      await expect(
        page.getByText(expectedMasked, { exact: true }),
        "masked token rendered in status row",
      ).toBeVisible({ timeout: 15_000 });

      // After a successful rotate, the primary button now reads "Rotate key"
      // (a token exists), and a Revoke button is present.
      await expect(
        page.getByRole("button", { name: "Rotate key" }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Revoke" }),
      ).toBeVisible();
    } finally {
      // Self-clean: if there was no token before this test, remove the one we made
      // so the workspace returns to its empty baseline and reruns stay deterministic.
      if (!before.has_token) {
        await revokeToken(page);
        const restored = await getTokenInfo(page);
        expect(
          restored.has_token,
          "token revoked back to baseline (was empty before)",
        ).toBe(false);
      }
    }
  });

  test("revoke the extension token via the UI: badge returns to 'No key' (self-restoring)", async ({
    page,
  }) => {
    const before = await getTokenInfo(page);

    // Ensure a token exists to revoke (create one via the API if needed).
    if (!before.has_token) {
      const gen = await page.request.post("/api/v1/settings/extension-token", {
        data: { name: "Default" },
      });
      expect(gen.ok(), "seed a token via API ok").toBeTruthy();
    }

    let restored = false;
    try {
      await page.goto(SETTINGS_URL);
      await expect(page.getByText("Loading...", { exact: true })).toHaveCount(
        0,
        { timeout: 15_000 },
      );

      // A token exists → Active badge + Revoke button.
      await expect(extCard(page).getByText("Active", { exact: true })).toBeVisible({
        timeout: 15_000,
      });
      const revokeBtn = page.getByRole("button", { name: "Revoke" });
      await expect(revokeBtn).toBeVisible();
      await revokeBtn.click();

      // After revoke + refetch the status flips to "No key" and the primary
      // button label becomes "Generate key"; Revoke disappears.
      await expect(
        extCard(page).getByText("No key", { exact: true }),
        "badge returns to No key after revoke",
      ).toBeVisible({ timeout: 15_000 });
      await expect(
        page.getByRole("button", { name: "Generate key" }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Revoke" }),
      ).toHaveCount(0);

      // Confirm via the API.
      const after = await getTokenInfo(page);
      expect(after.has_token, "token gone from API after revoke").toBe(false);

      // Restore: if a token existed before this test, mint a fresh one so the
      // workspace is left with a token (it can only ever hold one).
      if (before.has_token) {
        const regen = await page.request.post(
          "/api/v1/settings/extension-token",
          { data: { name: "Default" } },
        );
        expect(regen.ok(), "restore a token via API ok").toBeTruthy();
      }
      restored = true;
    } finally {
      if (!restored) {
        // Best-effort restore on failure: leave a token only if there was one.
        if (before.has_token) {
          await page.request.post("/api/v1/settings/extension-token", {
            data: { name: "Default" },
          });
        } else {
          await revokeToken(page);
        }
      }
    }
  });

  test("BYOK api-keys: GET lists supported providers as masked/never-raw status", async ({
    page,
  }) => {
    const res = await page.request.get("/api/v1/settings/api-keys");
    expect(res.ok(), "GET /settings/api-keys ok").toBeTruthy();
    const list = (await res.json()) as ApiKeyStatus[];
    expect(Array.isArray(list), "api-keys payload is an array").toBeTruthy();

    // Backend enumerates these supported providers (handler/settings.go).
    const providers = list.map((k) => k.provider).sort();
    expect(providers).toEqual(
      expect.arrayContaining(["glm", "openai", "voyage"]),
    );

    // Status entries never carry a raw key — only is_set + an optional masked_key.
    for (const k of list) {
      expect(typeof k.is_set).toBe("boolean");
      if (k.is_set) {
        expect(k.masked_key, `${k.provider} masked when set`).toBeTruthy();
        expect(k.masked_key!).toContain("...");
      } else {
        // an unset provider exposes no key material at all
        expect(k.masked_key ?? "").toBe("");
      }
    }
  });

  test("BYOK api-keys: PUT saves a fake key (masked persistence), then DELETE removes it", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    // A long, deterministic fake key. MaskKey keeps key[:4] + "..." + key[-4:]
    // (a 4-char prefix so short keys can't be fully reconstructed).
    const fakeKey = `sk-e2e-${uniqueSuffix}-FAKEKEYDONOTUSE-${uniqueSuffix}`;
    const expectedMasked = `${fakeKey.slice(0, 4)}...${fakeKey.slice(-4)}`;

    // Pick a provider that is currently EMPTY so we never clobber a real key.
    const before = (await (
      await page.request.get("/api/v1/settings/api-keys")
    ).json()) as ApiKeyStatus[];
    const target = before.find((k) => !k.is_set);
    expect(
      target,
      "at least one provider is unset to test against",
    ).toBeTruthy();
    const provider = target!.provider;

    let saved = false;
    try {
      // PUT saves the key; the response is the masked status for that provider.
      const put = await page.request.put("/api/v1/settings/api-keys", {
        data: { provider, api_key: fakeKey },
      });
      expect(put.ok(), "PUT /settings/api-keys ok").toBeTruthy();
      const putBody = (await put.json()) as ApiKeyStatus;
      expect(putBody.provider).toBe(provider);
      expect(putBody.is_set, "provider now set").toBe(true);
      expect(putBody.masked_key, "response masks the saved key").toBe(
        expectedMasked,
      );
      // The raw key is NEVER echoed back.
      expect(putBody.masked_key).not.toBe(fakeKey);
      saved = true;

      // Persistence: a subsequent GET reports the same provider as set + masked,
      // and the masked value matches the MaskKey contract (never the raw secret).
      const list = (await (
        await page.request.get("/api/v1/settings/api-keys")
      ).json()) as ApiKeyStatus[];
      const row = list.find((k) => k.provider === provider);
      expect(row, "saved provider present in list").toBeTruthy();
      expect(row!.is_set, "provider persisted as set").toBe(true);
      expect(row!.masked_key).toBe(expectedMasked);
      // The full raw secret is never exposed by the GET listing.
      expect(row!.masked_key).not.toBe(fakeKey);

      // DELETE removes the key; response reports the provider as no longer set.
      const del = await page.request.delete("/api/v1/settings/api-keys", {
        data: { provider },
      });
      expect(del.ok(), "DELETE /settings/api-keys ok").toBeTruthy();
      const delBody = (await del.json()) as ApiKeyStatus;
      expect(delBody.provider).toBe(provider);
      expect(delBody.is_set, "provider unset after delete").toBe(false);
      saved = false;

      // Persistence of deletion: GET reports the provider unset with no key.
      const after = (await (
        await page.request.get("/api/v1/settings/api-keys")
      ).json()) as ApiKeyStatus[];
      const afterRow = after.find((k) => k.provider === provider);
      expect(afterRow, "provider still listed after delete").toBeTruthy();
      expect(afterRow!.is_set, "provider back to unset").toBe(false);
      expect(afterRow!.masked_key ?? "").toBe("");
    } finally {
      // Self-clean: ensure the provider is left unset (it was unset at the start).
      if (saved) {
        await page.request.delete("/api/v1/settings/api-keys", {
          data: { provider },
        });
      }
    }
  });

  test("BYOK api-keys: unsupported provider and empty key are rejected", async ({
    page,
  }) => {
    // Unsupported provider → 400.
    const badProvider = await page.request.put("/api/v1/settings/api-keys", {
      data: { provider: "not-a-real-provider", api_key: "whatever" },
    });
    expect(
      badProvider.status(),
      "unsupported provider rejected with 400",
    ).toBe(400);

    // Empty key for a supported provider → 400 (no key written).
    const emptyKey = await page.request.put("/api/v1/settings/api-keys", {
      data: { provider: "openai", api_key: "" },
    });
    expect(emptyKey.status(), "empty api_key rejected with 400").toBe(400);

    // Sanity: openai remains unset (the rejected PUT wrote nothing).
    const list = (await (
      await page.request.get("/api/v1/settings/api-keys")
    ).json()) as ApiKeyStatus[];
    const openai = list.find((k) => k.provider === "openai");
    expect(openai, "openai listed").toBeTruthy();
    expect(openai!.is_set, "openai still unset after rejected writes").toBe(
      false,
    );
  });
});

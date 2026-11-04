import { test, expect } from "@playwright/test";

/**
 * Built-in auth: email/password (+ verification and reset when Resend is
 * configured) and Google/GitHub OAuth when their client IDs are set.
 *
 * The e2e stack has no RESEND_API_KEY, so email verification is off and new
 * password accounts are auto-verified. OAuth tests skip unless configured.
 *
 * UI:  dashboard/src/components/auth/auth-panel.tsx (/login, /register),
 *      routes/_auth/{forgot,reset}-password.tsx
 * API: backend/internal/api/auth_routes.go
 */

test.use({ storageState: { cookies: [], origins: [] } });

const SEEDED = { email: "e2e-tester@leadecho.test", password: "e2e-password-123" };

type Providers = { google: boolean; github: boolean; password: boolean; email_verification: boolean };

test.describe("auth (fresh, logged-out context)", () => {
  test("providers endpoint describes enabled sign-in methods", async ({ request }) => {
    const res = await request.get("/api/v1/auth/providers");
    expect(res.ok()).toBeTruthy();
    const p: Providers = await res.json();
    expect(p.password).toBe(true);
    expect(p.email_verification).toBe(false); // no Resend key on the e2e stack
  });

  test("login page renders the form and only the configured OAuth buttons", async ({ page, request }) => {
    const p: Providers = await (await request.get("/api/v1/auth/providers")).json();
    await page.goto("/login");
    await expect(page.getByRole("heading", { name: "Welcome to LeadEcho" })).toBeVisible();
    await expect(page.getByPlaceholder("Email")).toBeVisible();
    await expect(page.getByPlaceholder("Password")).toBeVisible();
    await expect(page.getByRole("link", { name: "Forgot password?" })).toBeVisible();
    await expect(page.getByRole("button", { name: /Continue with Google/ })).toHaveCount(p.google ? 1 : 0);
    await expect(page.getByRole("button", { name: /Continue with GitHub/ })).toHaveCount(p.github ? 1 : 0);
  });

  test("register creates an account and signs in", async ({ page }) => {
    const email = `e2e-reg-${Date.now()}@leadecho.test`;
    await page.goto("/register");
    await page.getByPlaceholder("Name").fill("E2E Reg");
    await page.getByPlaceholder("Email").fill(email);
    await page.getByPlaceholder("Password (min 8 characters)").fill("e2e-register-pass-123");
    const [resp] = await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/auth/register") && r.request().method() === "POST"),
      page.getByRole("button", { name: "Create account" }).click(),
    ]);
    expect(resp.status()).toBe(201);
    const me = await page.request.get("/api/v1/auth/me");
    expect((await me.json()).email).toBe(email);
  });

  test("register rejects duplicate email and short / over-long passwords", async ({ request }) => {
    expect((await request.post("/api/v1/auth/register", { data: { ...SEEDED, name: "Dup" } })).status()).toBe(409);
    const base = { email: `e2e-pw-${Date.now()}@leadecho.test`, name: "Pw" };
    expect((await request.post("/api/v1/auth/register", { data: { ...base, password: "short" } })).status()).toBe(400);
    expect((await request.post("/api/v1/auth/register", { data: { ...base, password: "x".repeat(73) } })).status()).toBe(400);
  });

  test("login signs the seeded account in", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("Email").fill(SEEDED.email);
    await page.getByPlaceholder("Password").fill(SEEDED.password);
    const [resp] = await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/auth/login") && r.request().method() === "POST"),
      page.getByRole("button", { name: "Sign in", exact: true }).click(),
    ]);
    expect(resp.status()).toBe(200);
    await page.waitForURL(/\/inbox/);
    const me = await page.request.get("/api/v1/auth/me");
    expect((await me.json()).email).toBe(SEEDED.email);
  });

  test("login with bad credentials shows an error and no session", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("Email").fill(`nobody-${Date.now()}@leadecho.test`);
    await page.getByPlaceholder("Password").fill("wrong-password");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText(/invalid email or password/i);
    expect((await page.request.get("/api/v1/auth/me")).ok()).toBe(false);
  });

  test("forgot-password never reveals whether an account exists", async ({ page, request }) => {
    const known = await request.post("/api/v1/auth/forgot-password", { data: { email: SEEDED.email } });
    const unknown = await request.post("/api/v1/auth/forgot-password", { data: { email: `ghost-${Date.now()}@x.test` } });
    expect(known.status()).toBe(200);
    expect(await known.json()).toEqual(await unknown.json());

    await page.goto("/forgot-password");
    await page.getByPlaceholder("Email").fill(SEEDED.email);
    await page.getByRole("button", { name: "Send reset link" }).click();
    await expect(page.getByText(/reset link is on its way/i)).toBeVisible();
  });

  test("reset-password rejects an invalid token", async ({ page, request }) => {
    const res = await request.post("/api/v1/auth/reset-password", { data: { token: "forged", password: "new-password-123" } });
    expect(res.status()).toBe(400);
    expect((await res.json()).code).toBe("invalid_token");

    await page.goto("/reset-password?token=forged");
    await page.getByPlaceholder("New password (min 8 characters)").fill("new-password-123");
    await page.getByPlaceholder("Confirm new password").fill("new-password-123");
    await page.getByRole("button", { name: "Update password" }).click();
    await expect(page.getByRole("alert")).toContainText(/invalid or expired/i);
  });

  test("verify with a bad token redirects to /login with a friendly error", async ({ page }) => {
    await page.goto("/api/v1/auth/verify?token=forged");
    await expect(page).toHaveURL(/\/login\?error=invalid_token/);
    await expect(page.getByRole("alert")).toContainText(/invalid or has expired/i);
  });

  test("OAuth callbacks reject a forged state", async ({ page, request }) => {
    const p: Providers = await (await request.get("/api/v1/auth/providers")).json();
    const provider = p.google ? "google" : p.github ? "github" : "";
    test.skip(!provider, "no OAuth provider configured on this stack");
    await page.goto(`/api/v1/auth/${provider}/callback?code=x&state=forged`);
    await expect(page).toHaveURL(/\/login\?error=invalid_state/);
  });
});

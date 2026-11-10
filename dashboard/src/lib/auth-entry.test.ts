import { test } from "node:test";
import assert from "node:assert/strict";
import { authView } from "./auth-entry.ts";

const oidc = { oidc: true, oidc_idps: ["google", "github"] };
const noOidc = { oidc: false };

test("authView: loading until providers arrive", () => {
  assert.deepEqual(authView(undefined, "", "login"), { kind: "loading" });
});

test("authView: oidc off keeps the in-app panel, whatever the URL says", () => {
  for (const search of ["", "?legacy=1", "?error=auth_failed", "?idp=google"]) {
    assert.deepEqual(authView(noOidc, search, "login"), { kind: "panel" });
    assert.deepEqual(authView(noOidc, search, "register"), { kind: "panel" });
  }
});

test("authView: oidc on redirects straight to Keycloak", () => {
  assert.deepEqual(authView(oidc, "", "login"), { kind: "redirect", to: "/api/v1/auth/oidc" });
  assert.deepEqual(authView(oidc, "", "register"), { kind: "redirect", to: "/api/v1/auth/oidc?screen=register" });
});

test("authView: only known idps pass through", () => {
  assert.deepEqual(authView(oidc, "?idp=github", "login"), { kind: "redirect", to: "/api/v1/auth/oidc?idp=github" });
  assert.deepEqual(authView(oidc, "?idp=google", "register"), {
    kind: "redirect",
    to: "/api/v1/auth/oidc?idp=google&screen=register",
  });
  assert.deepEqual(authView(oidc, "?idp=evil%26x%3D1", "login"), { kind: "redirect", to: "/api/v1/auth/oidc" });
  assert.deepEqual(authView({ oidc: true }, "?idp=google", "login"), { kind: "redirect", to: "/api/v1/auth/oidc" });
});

test("authView: ?error= shows the error with a retry instead of looping", () => {
  assert.deepEqual(authView(oidc, "?error=auth_failed", "login"), {
    kind: "error",
    code: "auth_failed",
    retry: "/api/v1/auth/oidc",
  });
  assert.deepEqual(authView(oidc, "?error=oauth_denied&legacy=1", "register"), {
    kind: "error",
    code: "oauth_denied",
    retry: "/api/v1/auth/oidc?screen=register",
  });
});

test("authView: legacy password form on /login only", () => {
  assert.deepEqual(authView(oidc, "?legacy=1", "login"), { kind: "legacy" });
  assert.deepEqual(authView(oidc, "?verified=1", "login"), { kind: "legacy" });
  assert.deepEqual(authView(oidc, "?legacy=1", "register"), {
    kind: "redirect",
    to: "/api/v1/auth/oidc?screen=register",
  });
  assert.deepEqual(authView(oidc, "?legacy=0", "login"), { kind: "redirect", to: "/api/v1/auth/oidc" });
});

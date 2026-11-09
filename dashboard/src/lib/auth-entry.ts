import type { AuthProviders } from "./api";

export type AuthMode = "login" | "register";

/** What /login and /register render. */
export type AuthView =
  | { kind: "loading" }
  | { kind: "redirect"; to: string }
  | { kind: "error"; code: string; retry: string }
  | { kind: "legacy" }
  | { kind: "panel" };

const OIDC_START = "/api/v1/auth/oidc";

/**
 * With OIDC (Keycloak) on, its hosted page is the one sign-in screen, so we go
 * straight there — except for ?error= (show it, offer a retry), and on /login
 * for ?legacy=1 / ?verified=1 (the old password form for pre-Keycloak accounts).
 * Without OIDC, the in-app password + OAuth panel. `providers` undefined = still loading.
 */
export function authView(
  providers: Pick<AuthProviders, "oidc" | "oidc_idps"> | undefined,
  search: string,
  mode: AuthMode,
): AuthView {
  if (!providers) return { kind: "loading" };
  if (!providers.oidc) return { kind: "panel" };

  const params = new URLSearchParams(search);
  const target = new URLSearchParams();
  const idp = params.get("idp");
  if (idp && providers.oidc_idps?.includes(idp)) target.set("idp", idp);
  if (mode === "register") target.set("screen", "register");
  const qs = target.toString();
  const to = qs ? `${OIDC_START}?${qs}` : OIDC_START;

  const error = params.get("error");
  if (error) return { kind: "error", code: error, retry: to };
  if (mode === "login" && (params.get("legacy") === "1" || params.get("verified") === "1")) {
    return { kind: "legacy" };
  }
  return { kind: "redirect", to };
}

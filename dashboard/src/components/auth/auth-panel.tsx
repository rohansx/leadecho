import { useEffect, useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { AuthError, getAuthProviders, loginWithEmail, registerWithEmail, resendVerification } from "@/lib/api";
import type { AuthProviders } from "@/lib/api";
import { authView, type AuthMode } from "@/lib/auth-entry";
import { FormError, OAuthButtons, UrlError } from "./auth-shared";
import { errorMessage } from "./errors";

function CheckEmail({ email }: { email: string }) {
  const [sent, setSent] = useState(false);
  return (
    <div className="space-y-3 text-center">
      <Text as="p" className="text-sm">
        We sent a verification link to <strong>{email}</strong>. Click it to finish signing in.
      </Text>
      <Button
        variant="outline"
        size="sm"
        disabled={sent}
        onClick={() => resendVerification(email).finally(() => setSent(true))}
      >
        {sent ? "Sent" : "Resend email"}
      </Button>
    </div>
  );
}

function Redirecting() {
  return (
    <Text as="p" role="status" className="text-center text-sm text-muted-foreground">
      Redirecting to sign in…
    </Text>
  );
}

/** OIDC came back with ?error=: say why, and let the user go again (no auto-loop). */
function OidcError({ retry }: { retry: string }) {
  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>Sign-in didn't finish</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <UrlError />
        <Button className="w-full" size="lg" onClick={() => window.location.replace(retry)}>
          Try again
        </Button>
      </CardContent>
    </Card>
  );
}

interface PasswordCardProps {
  mode: AuthMode;
  /** OIDC is on: only the pre-Keycloak password login, no OAuth buttons or sign-up link. */
  legacy?: boolean;
  providers?: AuthProviders;
}

function PasswordCard({ mode, legacy, providers }: PasswordCardProps) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [unverified, setUnverified] = useState(false);
  const [checkEmail, setCheckEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const isRegister = mode === "register";

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setUnverified(false);
    setLoading(true);
    try {
      if (isRegister) {
        const res = await registerWithEmail(email, password, name);
        if ("verification_required" in res) {
          setCheckEmail(res.email);
          return;
        }
        window.location.href = "/onboarding";
      } else {
        await loginWithEmail(email, password);
        window.location.href = "/inbox";
      }
    } catch (err: unknown) {
      setUnverified(err instanceof AuthError && err.code === "email_unverified");
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{isRegister ? "Create your account" : legacy ? "Sign in with password" : "Welcome to LeadEcho"}</CardTitle>
        <CardDescription>
          {isRegister
            ? "Start catching buying signals"
            : legacy
              ? "For accounts created with an email and password"
              : "Sign in to start catching buying signals"}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <UrlError />
        {checkEmail ? (
          <CheckEmail email={checkEmail} />
        ) : (
          <>
            <form onSubmit={submit} className="space-y-3">
              {isRegister && (
                <Input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} required autoComplete="name" />
              )}
              <Input type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} required autoComplete="email" />
              <Input
                type="password"
                placeholder={isRegister ? "Password (min 8 characters)" : "Password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={isRegister ? 8 : undefined}
                maxLength={72}
                autoComplete={isRegister ? "new-password" : "current-password"}
              />
              <FormError message={error} />
              {unverified && (
                <Button type="button" variant="link" size="sm" onClick={() => resendVerification(email).then(() => setCheckEmail(email))}>
                  Resend verification email
                </Button>
              )}
              <Button type="submit" className="w-full" size="lg" disabled={loading}>
                {loading ? "Please wait..." : isRegister ? "Create account" : "Sign in"}
              </Button>
              {!isRegister && (
                <div className="text-right">
                  <Link to="/forgot-password" className="text-xs text-muted-foreground underline hover:text-foreground">
                    Forgot password?
                  </Link>
                </div>
              )}
            </form>
            {!legacy && <OAuthButtons providers={providers} />}
          </>
        )}
      </CardContent>
      <CardFooter className="justify-center">
        <Text as="p" className="text-xs text-muted-foreground text-center">
          {legacy ? (
            <a href="/login" className="underline hover:text-foreground">
              Back to sign in
            </a>
          ) : (
            <>
              {isRegister ? "Already have an account? " : "Don't have an account? "}
              <Link to={isRegister ? "/login" : "/register"} className="underline hover:text-foreground">
                {isRegister ? "Sign in" : "Register"}
              </Link>
            </>
          )}
        </Text>
      </CardFooter>
    </Card>
  );
}

export function AuthPanel({ mode }: { mode: AuthMode }) {
  const { data: providers, isError } = useQuery({ queryKey: ["auth-providers"], queryFn: getAuthProviders });
  const search = useRouterState({ select: (s) => s.location.searchStr });
  // ponytail: providers fetch failed → fall back to the in-app panel rather than spin forever.
  const view = authView(isError ? { oidc: false } : providers, search, mode);
  const redirectTo = view.kind === "redirect" ? view.to : "";

  useEffect(() => {
    // replace(), not href: Back from Keycloak shouldn't land here and bounce again.
    if (redirectTo) window.location.replace(redirectTo);
  }, [redirectTo]);

  switch (view.kind) {
    case "loading":
    case "redirect":
      return <Redirecting />;
    case "error":
      return <OidcError retry={view.retry} />;
    case "legacy":
      return <PasswordCard mode="login" legacy />;
    case "panel":
      return <PasswordCard mode={mode} providers={providers} />;
  }
}

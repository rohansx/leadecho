import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { AuthError, getAuthProviders, loginWithEmail, registerWithEmail, resendVerification } from "@/lib/api";
import { FormError, OAuthButtons, UrlError } from "./auth-shared";
import { errorMessage } from "./errors";

type Mode = "login" | "register";

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

export function AuthPanel({ mode }: { mode: Mode }) {
  const { data: providers } = useQuery({ queryKey: ["auth-providers"], queryFn: getAuthProviders });
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [unverified, setUnverified] = useState(false);
  const [checkEmail, setCheckEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [usePassword, setUsePassword] = useState(false);
  const isRegister = mode === "register";
  // Logto hosts sign-in/sign-up (and social logins); the password form stays for older accounts.
  const hosted = !!providers?.logto && !usePassword;

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
        <CardTitle>{isRegister ? "Create your account" : "Welcome to LeadEcho"}</CardTitle>
        <CardDescription>
          {isRegister ? "Start catching buying signals" : "Sign in to start catching buying signals"}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <UrlError />
        {hosted ? (
          <div className="space-y-3">
            <a href={`/api/v1/auth/logto${isRegister ? "?screen=register" : ""}`} className="block">
              <Button className="w-full" size="lg" type="button">
                {isRegister ? "Create account" : "Continue"}
              </Button>
            </a>
            {!isRegister && (
              <button type="button" className="w-full text-center text-xs text-muted-foreground underline hover:text-foreground" onClick={() => setUsePassword(true)}>
                Sign in with email and password
              </button>
            )}
          </div>
        ) : checkEmail ? (
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
            <OAuthButtons providers={providers} />
          </>
        )}
      </CardContent>
      <CardFooter className="justify-center">
        <Text as="p" className="text-xs text-muted-foreground text-center">
          {isRegister ? "Already have an account? " : "Don't have an account? "}
          <Link to={isRegister ? "/login" : "/register"} className="underline hover:text-foreground">
            {isRegister ? "Sign in" : "Register"}
          </Link>
        </Text>
      </CardFooter>
    </Card>
  );
}

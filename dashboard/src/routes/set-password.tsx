// set-password: the step after claiming an account (GET /api/v1/auth/claim).
// The account's temporary password was dropped on claim; the new owner picks
// one here, or skips and signs in later with Google or "Forgot password".
import { createFileRoute, Navigate, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Logo } from "@/components/marketing/logo";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { FormError } from "@/components/auth/auth-shared";
import { errorMessage } from "@/components/auth/errors";
import { request } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/set-password")({ component: SetPasswordPage });

function SetPasswordPage() {
  const { user, loading } = useAuth();
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  if (loading) return null;
  if (!user) return <Navigate to="/login" replace />;

  const toDashboard = () => navigate({ to: "/inbox", replace: true });

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password !== confirm) {
      setError("Passwords don't match");
      return;
    }
    setError("");
    setSaving(true);
    try {
      await request("/auth/claim/password", { method: "POST", body: JSON.stringify({ password }) });
      toDashboard();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <div className="w-full max-w-md">
        <div className="mb-8 flex justify-center">
          <Logo className="text-2xl [&>svg]:size-8" />
        </div>
        <Card className="w-full">
          <CardHeader>
            <CardTitle>Your account is ready</CardTitle>
            <CardDescription>
              Signed in as {user.email}. Choose a password so you can sign in again.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit} className="space-y-3">
              <Input type="password" placeholder="Password (min 8 characters)" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} maxLength={72} autoComplete="new-password" />
              <Input type="password" placeholder="Confirm password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required autoComplete="new-password" />
              <FormError message={error} />
              <Button type="submit" className="w-full" size="lg" disabled={saving}>
                {saving ? "Saving..." : "Set password and continue"}
              </Button>
            </form>
          </CardContent>
          <CardFooter className="justify-center">
            <button type="button" onClick={toDashboard} className="text-xs text-muted-foreground underline hover:text-foreground">
              <Text as="span">Skip for now</Text>
            </button>
          </CardFooter>
        </Card>
      </div>
    </div>
  );
}

import { createFileRoute, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { resetPassword } from "@/lib/api";
import { FormError } from "@/components/auth/auth-shared";
import { errorMessage } from "@/components/auth/errors";

export const Route = createFileRoute("/_auth/reset-password")({
  component: ResetPasswordPage,
});

// Read the token once (lazy state init), then drop it from the address bar so
// it isn't left in history or shared by copy-paste.
function readToken(): string {
  return new URLSearchParams(window.location.search).get("token") ?? "";
}

function ResetPasswordPage() {
  const [token] = useState(readToken);
  useEffect(() => {
    if (window.location.search.includes("token=")) {
      window.history.replaceState(null, "", window.location.pathname);
    }
  }, []);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password !== confirm) {
      setError("Passwords don't match");
      return;
    }
    setError("");
    setLoading(true);
    try {
      await resetPassword(token, password);
      setDone(true);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>Choose a new password</CardTitle>
        <CardDescription>This signs you out everywhere else.</CardDescription>
      </CardHeader>
      <CardContent>
        {!token ? (
          <Text as="p" className="text-sm text-destructive">This reset link is missing its token. Request a new one.</Text>
        ) : done ? (
          <Text as="p" className="text-sm">Your password was updated. You can sign in now.</Text>
        ) : (
          <form onSubmit={submit} className="space-y-3">
            <Input type="password" placeholder="New password (min 8 characters)" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} maxLength={72} autoComplete="new-password" />
            <Input type="password" placeholder="Confirm new password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required autoComplete="new-password" />
            <FormError message={error} />
            <Button type="submit" className="w-full" size="lg" disabled={loading}>
              {loading ? "Saving..." : "Update password"}
            </Button>
          </form>
        )}
      </CardContent>
      <CardFooter className="justify-center">
        <Link to={done ? "/login" : "/forgot-password"} className="text-xs text-muted-foreground underline hover:text-foreground">
          {done ? "Sign in" : "Request a new link"}
        </Link>
      </CardFooter>
    </Card>
  );
}

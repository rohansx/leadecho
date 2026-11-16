// Operator console: temporary accounts built for prospects, and the Lobby
// claim links that hand them over. Sign in as the temp account (its email and
// password) to set it up; claiming swaps in the prospect's verified email and
// drops the temporary password.
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { OPS_KEYS, createClaimLink, createTempAccount, listTempAccounts, type ClaimLink, type TempAccount } from "@/lib/ops-api";

function CreateTempAccountForm() {
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [skipVerification, setSkipVerification] = useState(true);
  const create = useMutation({
    mutationFn: () => createTempAccount({ email, password, name, skip_verification: skipVerification }),
    onSuccess: () => {
      setEmail("");
      setPassword("");
      setName("");
      qc.invalidateQueries({ queryKey: ["ops"] });
    },
  });
  return (
    <form
      className="flex flex-wrap items-center gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <Input className="h-8 w-64" type="email" placeholder="demo+acme@yourdomain.com" value={email} onChange={(e) => setEmail(e.target.value)} required autoComplete="off" />
      <Input className="h-8 w-44" type="password" placeholder="Temp password (8+)" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} maxLength={72} autoComplete="new-password" />
      <Input className="h-8 w-40" placeholder="Name (optional)" value={name} onChange={(e) => setName(e.target.value)} maxLength={100} />
      <label className="flex items-center gap-1.5 text-xs text-muted-foreground" title="Mark the placeholder email verified, so signing in doesn't wait for a verification email">
        <input type="checkbox" checked={skipVerification} onChange={(e) => setSkipVerification(e.target.checked)} />
        Skip email verification
      </label>
      <Button size="sm" type="submit" disabled={create.isPending}>
        {create.isPending ? "Creating..." : "Create temp account"}
      </Button>
      {create.error && <Text as="p" className="w-full text-sm text-destructive">{create.error.message}</Text>}
      {create.data && (
        <Text as="p" className="w-full text-sm text-muted-foreground">
          Created {create.data.email}.{" "}
          {create.data.keycloak
            ? "Sign out, then sign in with it on the normal sign-in page to set the account up."
            : "Sign out, then sign in with it at /login?legacy=1 to set the account up."}
        </Text>
      )}
    </form>
  );
}

function ClaimCell({ account, lobbyConfigured }: { account: TempAccount; lobbyConfigured: boolean }) {
  const [label, setLabel] = useState("");
  const [lockEmail, setLockEmail] = useState("");
  const [copied, setCopied] = useState(false);
  const claim = useMutation<ClaimLink>({ mutationFn: () => createClaimLink(account.user_id, { label, email: lockEmail }) });
  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };
  if (!lobbyConfigured) return <span className="text-xs text-muted-foreground">Set LOBBY_SECRET_KEY</span>;
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <Input className="h-7 w-40 px-2 text-xs" placeholder="Name at Company" value={label} onChange={(e) => setLabel(e.target.value)} maxLength={120} />
        <Input className="h-7 w-48 px-2 text-xs" type="email" placeholder="Lock to email (optional)" value={lockEmail} onChange={(e) => setLockEmail(e.target.value)} />
        <Button size="sm" variant="outline" className="h-7 px-2 text-xs" disabled={claim.isPending} onClick={() => { setCopied(false); claim.mutate(); }} title="A new link revokes this account's previous one">
          {claim.isPending ? "Creating..." : "Create claim link"}
        </Button>
      </div>
      {claim.error && <p className="text-xs text-destructive">{claim.error.message}</p>}
      {claim.data && (
        <div className="flex items-center gap-1.5">
          <Input readOnly className="h-7 w-80 px-2 font-mono text-xs" value={claim.data.claim_url} onFocus={(e) => e.target.select()} />
          <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => copy(claim.data.claim_url)}>
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      )}
    </div>
  );
}

export function TempAccountsCard() {
  const accounts = useQuery({ queryKey: OPS_KEYS.tempAccounts, queryFn: listTempAccounts, retry: false });
  const rows = accounts.data?.data ?? [];
  const lobbyConfigured = accounts.data?.lobby_configured ?? false;
  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <div>
          <Text as="p" className="font-medium">Temporary accounts ({rows.length})</Text>
          <Text as="p" className="text-xs text-muted-foreground">
            Build an account for a prospect under a placeholder email, then send them a claim link. Claiming swaps in their verified email and removes the temporary password, API keys and extension token. A link works once: if a claim is refused (their email already has an account), create a new one.
          </Text>
        </div>
        <CreateTempAccountForm />
        {accounts.error && <Text as="p" className="text-sm text-destructive">{accounts.error.message}</Text>}
        {rows.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-[13px]">
              <thead className="text-xs text-muted-foreground">
                <tr className="border-b border-border">
                  <th className="py-2 pr-3 font-medium">Account</th>
                  <th className="py-2 pr-3 font-medium">Created</th>
                  <th className="py-2 font-medium">Claim link</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((a) => (
                  <tr key={a.user_id} className="border-b border-border/60 align-top">
                    <td className="py-2 pr-3">
                      <div className="font-medium">{a.email}</div>
                      <div className="text-xs text-muted-foreground">{a.workspace_name}</div>
                    </td>
                    <td className="py-2 pr-3 text-xs text-muted-foreground">{new Date(a.created_at).toLocaleDateString()}</td>
                    <td className="py-2">
                      <ClaimCell account={a} lobbyConfigured={lobbyConfigured} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

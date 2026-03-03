import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Text } from "@/components/ui/text";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Settings as SettingsIcon, Sparkles, Puzzle, Copy, Check, AlertTriangle } from "lucide-react";
import { getExtensionToken, rotateExtensionToken, revokeExtensionToken } from "@/lib/api";

export const Route = createFileRoute("/_dashboard/settings")({
  component: SettingsPage,
});

function ChromeExtensionCard() {
  const queryClient = useQueryClient();
  const [newToken, setNewToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const { data: tokenInfo, isLoading } = useQuery({
    queryKey: ["extension-token"],
    queryFn: getExtensionToken,
  });

  const rotate = useMutation({
    mutationFn: () => rotateExtensionToken(),
    onSuccess: (data) => {
      setNewToken(data.token);
      queryClient.invalidateQueries({ queryKey: ["extension-token"] });
    },
  });

  const revoke = useMutation({
    mutationFn: revokeExtensionToken,
    onSuccess: () => {
      setNewToken(null);
      queryClient.invalidateQueries({ queryKey: ["extension-token"] });
    },
  });

  function copyToken() {
    if (!newToken) return;
    navigator.clipboard.writeText(newToken).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  const hasToken = tokenInfo?.has_token;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
            <Puzzle className="h-5 w-5 text-primary-foreground" />
          </div>
          <div>
            <CardTitle>Chrome Extension</CardTitle>
            <CardDescription>
              Passively capture signals while you browse Reddit, X, LinkedIn, and HN.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Token status */}
        <div className="flex items-center justify-between p-3 rounded border-2 border-border bg-background">
          <div className="flex items-center gap-2">
            <Badge variant={hasToken ? "surface" : "outline"} size="sm">
              {isLoading ? "Loading..." : hasToken ? "Active" : "No key"}
            </Badge>
            {hasToken && tokenInfo?.masked_token && (
              <Text as="p" className="text-sm font-mono text-muted-foreground">
                {tokenInfo.masked_token}
              </Text>
            )}
            {hasToken && tokenInfo?.last_used_at && (
              <Text as="p" className="text-xs text-muted-foreground">
                Last used {new Date(tokenInfo.last_used_at).toLocaleDateString()}
              </Text>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => rotate.mutate()}
              disabled={rotate.isPending}
              className="px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background hover:bg-accent transition-colors disabled:opacity-50"
            >
              {rotate.isPending ? "Generating..." : hasToken ? "Rotate key" : "Generate key"}
            </button>
            {hasToken && (
              <button
                onClick={() => revoke.mutate()}
                disabled={revoke.isPending}
                className="px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-50"
              >
                Revoke
              </button>
            )}
          </div>
        </div>

        {/* New token reveal — shown once after generation */}
        {newToken && (
          <div className="p-3 rounded border-2 border-amber-500/50 bg-amber-500/5 space-y-2">
            <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              <Text as="p" className="text-sm font-medium">
                Copy this key now — it won't be shown again.
              </Text>
            </div>
            <div className="flex items-center gap-2">
              <input
                readOnly
                value={newToken}
                className="flex-1 px-2 py-1.5 text-sm font-mono rounded border-2 border-border bg-background text-foreground"
              />
              <button
                onClick={copyToken}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background hover:bg-accent transition-colors"
              >
                {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
          </div>
        )}

        {/* Setup instructions */}
        <div className="space-y-2 pt-1">
          <Text as="p" className="text-sm font-medium">Setup</Text>
          <ol className="space-y-1 text-sm text-muted-foreground list-decimal list-inside">
            <li>Install the LeadEcho extension from the Chrome Web Store</li>
            <li>Click the extension icon → enter your backend URL</li>
            <li>Generate a key above and paste it into the extension popup</li>
            <li>Browse Reddit, X, LinkedIn, or HN — signals are captured automatically</li>
          </ol>
        </div>
      </CardContent>
    </Card>
  );
}

function SettingsPage() {
  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <Text as="h2">Settings</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Manage your account configuration.
        </Text>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Sparkles className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>AI Features</CardTitle>
              <CardDescription>
                Intent classification and reply drafting are included for all
                users.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 p-3 rounded border-2 border-border bg-background">
            <Badge variant="surface" size="sm">Included</Badge>
            <Text as="p" className="text-sm text-muted-foreground">
              AI-powered classify and draft reply are available on every mention in your Inbox.
            </Text>
          </div>
        </CardContent>
      </Card>

      <ChromeExtensionCard />

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <SettingsIcon className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Account</CardTitle>
              <CardDescription>
                Account settings and billing.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-8 text-center">
          <Text as="p" className="text-muted-foreground">
            Billing, plan upgrades, and custom API keys coming in Pro plan.
          </Text>
        </CardContent>
      </Card>
    </div>
  );
}

// mcp: Settings > API & MCP — workspace API keys, client snippets, tool list.
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Bot, Check, Copy } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import {
  createWorkspaceAPIKey,
  listAgentTools,
  listWorkspaceAPIKeys,
  revokeWorkspaceAPIKey,
  type APIKeyScope,
} from "@/lib/api";

const KEYS_QUERY = ["workspace-api-keys"];
const KEY_PLACEHOLDER = "le_live_YOUR_KEY";

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      size="sm"
      variant="outline"
      onClick={() =>
        navigator.clipboard.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        })
      }
    >
      {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
      {copied ? "Copied" : "Copy"}
    </Button>
  );
}

function Snippet({ label, code }: { label: string; code: string }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Text as="p" className="text-sm font-medium">{label}</Text>
        <CopyButton text={code} />
      </div>
      <pre className="overflow-x-auto rounded-lg border border-border bg-muted p-3 text-xs font-mono whitespace-pre">
        {code}
      </pre>
    </div>
  );
}

function snippets(mcpURL: string, key: string) {
  const header = `Authorization: Bearer ${key}`;
  return [
    {
      label: "Claude Code",
      code: `claude mcp add --transport http leadecho ${mcpURL} --header "${header}"`,
    },
    {
      label: "Cursor (~/.cursor/mcp.json) / Claude Desktop and other JSON-config clients",
      code: JSON.stringify(
        { mcpServers: { leadecho: { url: mcpURL, headers: { Authorization: `Bearer ${key}` } } } },
        null,
        2,
      ),
    },
    {
      label: "ChatGPT and other clients: server URL + header",
      code: `URL:    ${mcpURL}\nHeader: ${header}`,
    },
  ];
}

export function ApiMcpCard() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [write, setWrite] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);

  const { data: keys, isLoading } = useQuery({ queryKey: KEYS_QUERY, queryFn: listWorkspaceAPIKeys });
  const { data: tools } = useQuery({ queryKey: ["agent-tools"], queryFn: listAgentTools });

  const create = useMutation({
    mutationFn: () => {
      const scopes: APIKeyScope[] = write ? ["read", "write"] : ["read"];
      return createWorkspaceAPIKey(name.trim(), scopes);
    },
    onSuccess: (res) => {
      setSecret(res.secret);
      setName("");
      queryClient.invalidateQueries({ queryKey: KEYS_QUERY });
    },
  });
  const revoke = useMutation({
    mutationFn: revokeWorkspaceAPIKey,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEYS_QUERY }),
  });

  const mcpURL = `${window.location.origin}/api/mcp`;
  const activeKeys = (keys?.data ?? []).filter((k) => !k.revoked);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <Bot className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>API &amp; MCP</CardTitle>
            <CardDescription>
              Connect Claude, ChatGPT or Cursor to your leads, or call the public REST API.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        {/* Create */}
        <div className="space-y-2">
          <Text as="p" className="text-sm font-medium">Create an API key</Text>
          <div className="flex flex-wrap items-center gap-2">
            <Input
              className="flex-1 min-w-48"
              placeholder="Key name, e.g. Claude Code"
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} />
              Allow write (draft, queue, stage, CRM)
            </label>
            <Button size="sm" disabled={!name.trim() || create.isPending} onClick={() => create.mutate()}>
              {create.isPending ? "Creating..." : "Create key"}
            </Button>
          </div>
          {create.error && (
            <Text as="p" className="text-sm text-destructive">{create.error.message}</Text>
          )}
        </div>

        {/* Show once */}
        {secret && (
          <div className="p-3 rounded-lg border border-amber-500/50 bg-amber-500/5 space-y-2">
            <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              <Text as="p" className="text-sm font-medium">
                Copy this key now — it won't be shown again.
              </Text>
            </div>
            <div className="flex items-center gap-2">
              <input
                readOnly
                value={secret}
                className="flex-1 px-2 py-1.5 text-sm font-mono rounded-lg border border-border bg-background text-foreground"
              />
              <CopyButton text={secret} />
            </div>
          </div>
        )}

        {/* List */}
        <div className="space-y-2">
          <Text as="p" className="text-sm font-medium">Keys</Text>
          {isLoading ? (
            <Text as="p" className="text-sm text-muted-foreground">Loading...</Text>
          ) : activeKeys.length === 0 ? (
            <Text as="p" className="text-sm text-muted-foreground">No active keys.</Text>
          ) : (
            activeKeys.map((k) => (
              <div
                key={k.id}
                className="flex items-center justify-between gap-2 p-3 rounded-lg border border-border bg-background"
              >
                <div className="flex flex-wrap items-center gap-2 min-w-0">
                  <Text as="p" className="text-sm font-medium truncate">{k.name}</Text>
                  <Text as="p" className="text-sm font-mono text-muted-foreground">{k.prefix}…</Text>
                  {k.scopes.map((s) => (
                    <Badge key={s} variant="outline" size="sm">{s}</Badge>
                  ))}
                  <Text as="p" className="text-xs text-muted-foreground">
                    {k.last_used_at
                      ? `Last used ${new Date(k.last_used_at).toLocaleDateString()}`
                      : "Never used"}
                  </Text>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  disabled={revoke.isPending}
                  onClick={() => revoke.mutate(k.id)}
                >
                  Revoke
                </Button>
              </div>
            ))
          )}
        </div>

        {/* Connect */}
        <div className="space-y-3 pt-1">
          <Text as="p" className="text-sm font-medium">Connect Claude / ChatGPT / Cursor</Text>
          {snippets(mcpURL, secret ?? KEY_PLACEHOLDER).map((s) => (
            <Snippet key={s.label} label={s.label} code={s.code} />
          ))}
          <Text as="p" className="text-xs text-muted-foreground">
            REST: <code className="rounded-lg bg-muted px-1 py-0.5">GET /api/public/v1/mentions</code> and{" "}
            <code className="rounded-lg bg-muted px-1 py-0.5">/api/public/v1/leads</code> with the same
            Bearer header.
          </Text>
        </div>

        {/* Tools */}
        {tools && tools.data.length > 0 && (
          <div className="space-y-2 pt-1">
            <Text as="p" className="text-sm font-medium">Tools agents can call</Text>
            <ul className="space-y-1.5">
              {tools.data.map((t) => (
                <li key={t.name} className="flex items-start gap-2 text-sm">
                  <code className="shrink-0 rounded-lg bg-muted px-1.5 py-0.5 font-mono text-xs">{t.name}</code>
                  <Badge variant={t.scope === "write" ? "surface" : "outline"} size="sm" className="shrink-0">
                    {t.scope}
                  </Badge>
                  <span className="text-muted-foreground">{t.description}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

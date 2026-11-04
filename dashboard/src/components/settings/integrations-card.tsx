import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plug } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import {
  getIntegrations,
  saveIntegrations,
  testIntegrationWebhook,
  type IntegrationsUpdate,
} from "@/lib/api";

const QUERY_KEY = ["integrations"];

interface TokenRowProps {
  label: string;
  help: string;
  connected: boolean;
  pending: boolean;
  onSave: (token: string) => void;
}

function TokenRow({ label, help, connected, pending, onSave }: TokenRowProps) {
  const [value, setValue] = useState("");
  return (
    <div className="space-y-2 p-3 rounded border-2 border-border bg-background">
      <div className="flex items-center justify-between">
        <Text as="p" className="text-sm font-medium">{label}</Text>
        <Badge variant={connected ? "surface" : "outline"} size="sm">
          {connected ? "Connected" : "Not connected"}
        </Badge>
      </div>
      <Text as="p" className="text-xs text-muted-foreground">{help}</Text>
      <div className="flex gap-2">
        <Input
          type="password"
          autoComplete="off"
          placeholder={connected ? "Paste a new token to replace" : "Paste access token"}
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
        <Button
          size="sm"
          disabled={pending || !value.trim()}
          onClick={() => {
            onSave(value.trim());
            setValue("");
          }}
        >
          Save
        </Button>
        {connected && (
          <Button size="sm" variant="outline" disabled={pending} onClick={() => onSave("")}>
            Disconnect
          </Button>
        )}
      </div>
    </div>
  );
}

export function IntegrationsCard() {
  const queryClient = useQueryClient();
  const { data } = useQuery({ queryKey: QUERY_KEY, queryFn: getIntegrations });
  const [url, setUrl] = useState<string | null>(null);
  const [secret, setSecret] = useState("");

  const save = useMutation({
    mutationFn: (u: IntegrationsUpdate) => saveIntegrations(u),
    onSuccess: (next) => queryClient.setQueryData(QUERY_KEY, next),
  });
  const test = useMutation({ mutationFn: testIntegrationWebhook });

  const webhookUrl = url ?? data?.webhook_url ?? "";

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
            <Plug className="h-5 w-5 text-primary-foreground" />
          </div>
          <div>
            <CardTitle>CRM &amp; Webhooks</CardTitle>
            <CardDescription>
              Push lead.created, lead.stage_changed and conversion.recorded events to your CRM.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2 p-3 rounded border-2 border-border bg-background">
          <Text as="p" className="text-sm font-medium">Signed webhook</Text>
          <Text as="p" className="text-xs text-muted-foreground">
            HTTPS only. Each request carries{" "}
            <code className="rounded bg-muted px-1">X-LeadEcho-Signature: sha256=HMAC(secret, timestamp + "." + body)</code>{" "}
            and <code className="rounded bg-muted px-1">X-LeadEcho-Timestamp</code>. Works with Zapier, Make, n8n.
          </Text>
          <Input
            placeholder="https://hooks.example.com/leadecho"
            value={webhookUrl}
            onChange={(e) => setUrl(e.target.value)}
          />
          <Input
            type="password"
            autoComplete="off"
            placeholder={data?.webhook_secret_set ? "Signing secret set. Type to replace" : "Signing secret (recommended)"}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
          <div className="flex gap-2 items-center">
            <Button
              size="sm"
              disabled={save.isPending}
              onClick={() => {
                const u: IntegrationsUpdate = { webhook_url: webhookUrl.trim() };
                if (secret.trim()) u.webhook_secret = secret.trim();
                save.mutate(u, { onSuccess: () => { setSecret(""); setUrl(null); } });
              }}
            >
              Save webhook
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={test.isPending || !data?.webhook_url}
              onClick={() => test.mutate()}
            >
              {test.isPending ? "Sending…" : "Send test event"}
            </Button>
            {test.isSuccess && <Text as="span" className="text-xs text-green-600">Delivered</Text>}
          </div>
        </div>

        <TokenRow
          label="HubSpot"
          help="Private app token with crm.objects.contacts.write. New leads become contacts; stages map to lifecycle stage."
          connected={!!data?.hubspot_connected}
          pending={save.isPending}
          onSave={(t) => save.mutate({ hubspot_token: t })}
        />
        <TokenRow
          label="Attio"
          help="Workspace token with record and note read-write scopes. New leads become People; stage changes and conversions are logged as notes."
          connected={!!data?.attio_connected}
          pending={save.isPending}
          onSave={(t) => save.mutate({ attio_token: t })}
        />
      </CardContent>
    </Card>
  );
}

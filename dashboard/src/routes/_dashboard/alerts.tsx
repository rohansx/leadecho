import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  getWebhookConfig,
  saveWebhookConfig,
  testWebhook,
} from "@/lib/api";
import type { WebhookConfig } from "@/lib/types";
import { Bell, Mail, Send, MessageSquare, Hash } from "lucide-react";

export const Route = createFileRoute("/_dashboard/alerts")({
  component: AlertsPage,
});

function AlertsPage() {
  const queryClient = useQueryClient();
  const [slackUrl, setSlackUrl] = useState("");
  const [discordUrl, setDiscordUrl] = useState("");
  const [emailTo, setEmailTo] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [onNewMention, setOnNewMention] = useState(true);
  const [onHighIntent, setOnHighIntent] = useState(true);
  const [onNewLead, setOnNewLead] = useState(false);
  const [resendConfigured, setResendConfigured] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [testSuccess, setTestSuccess] = useState<string | null>(null);

  const { data: config } = useQuery({
    queryKey: ["webhook-config"],
    queryFn: getWebhookConfig,
  });

  // Sync form state from API data once loaded
  if (config && !loaded) {
    const c = config as WebhookConfig;
    setSlackUrl(c.slack_url || "");
    setDiscordUrl(c.discord_url || "");
    setEmailTo(c.email_to || "");
    setEnabled(c.enabled || false);
    setOnNewMention(c.on_new_mention ?? true);
    setOnHighIntent(c.on_high_intent ?? true);
    setOnNewLead(c.on_new_lead ?? false);
    setResendConfigured(c.resend_configured ?? false);
    setLoaded(true);
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      saveWebhookConfig({
        slack_url: slackUrl,
        discord_url: discordUrl,
        email_to: emailTo,
        enabled,
        on_new_mention: onNewMention,
        on_high_intent: onHighIntent,
        on_new_lead: onNewLead,
      }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["webhook-config"] }),
  });

  const showTestSuccess = (channel: string) => {
    setTestSuccess(channel);
    setTimeout(() => setTestSuccess(null), 3000);
  };

  const testSlack = useMutation({
    mutationFn: () => testWebhook("slack", slackUrl),
    onSuccess: () => showTestSuccess("Slack"),
  });
  const testDiscord = useMutation({
    mutationFn: () => testWebhook("discord", discordUrl),
    onSuccess: () => showTestSuccess("Discord"),
  });
  const testEmail = useMutation({
    mutationFn: () => testWebhook("email", emailTo),
    onSuccess: () => showTestSuccess("Email"),
  });

  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <Text as="h2">Alerts</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Get notified on Slack, Discord, or email when new mentions and signals arrive.
        </Text>
      </div>

      {/* Master toggle */}
      <Card>
        <CardContent className="py-4">
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className="w-4 h-4 accent-primary"
            />
            <div>
              <Text as="span" className="text-sm font-medium">
                Enable notifications
              </Text>
              <Text as="p" className="text-xs text-muted-foreground">
                Master switch for all alert channels below.
              </Text>
            </div>
          </label>
        </CardContent>
      </Card>

      {/* Slack */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Hash className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Slack</CardTitle>
              <CardDescription>
                Send alerts to a Slack channel via incoming webhook.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          <Text as="p" className="text-sm font-medium">Webhook URL</Text>
          <div className="flex gap-2">
            <Input
              placeholder="https://hooks.slack.com/services/..."
              value={slackUrl}
              onChange={(e) => setSlackUrl(e.target.value)}
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => testSlack.mutate()}
              disabled={!slackUrl || testSlack.isPending}
            >
              <Send className="h-3.5 w-3.5 mr-1" />
              Test
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Discord */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <MessageSquare className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Discord</CardTitle>
              <CardDescription>
                Send alerts to a Discord channel via webhook.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          <Text as="p" className="text-sm font-medium">Webhook URL</Text>
          <div className="flex gap-2">
            <Input
              placeholder="https://discord.com/api/webhooks/..."
              value={discordUrl}
              onChange={(e) => setDiscordUrl(e.target.value)}
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => testDiscord.mutate()}
              disabled={!discordUrl || testDiscord.isPending}
            >
              <Send className="h-3.5 w-3.5 mr-1" />
              Test
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Email */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Mail className="h-5 w-5 text-primary-foreground" />
            </div>
            <div className="flex items-center gap-2">
              <div>
                <CardTitle>Email</CardTitle>
                <CardDescription>
                  Receive alert digests via email powered by Resend.
                </CardDescription>
              </div>
            </div>
          </div>
          {resendConfigured ? (
            <Badge variant="surface" size="sm" className="ml-auto">Resend connected</Badge>
          ) : (
            <Badge variant="outline" size="sm" className="text-orange-600 border-orange-300 ml-auto">
              Set RESEND_API_KEY in .env
            </Badge>
          )}
        </CardHeader>
        <CardContent className="space-y-2">
          <Text as="p" className="text-sm font-medium">Recipient Email</Text>
          <div className="flex gap-2">
            <Input
              placeholder="alerts@yourcompany.com"
              type="email"
              value={emailTo}
              onChange={(e) => setEmailTo(e.target.value)}
              disabled={!resendConfigured}
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => testEmail.mutate()}
              disabled={!emailTo || !resendConfigured || testEmail.isPending}
            >
              <Mail className="h-3.5 w-3.5 mr-1" />
              Test
            </Button>
          </div>
          {!resendConfigured && (
            <Text as="p" className="text-xs text-muted-foreground">
              Add your Resend API key to .env to enable email alerts. Free tier: 3,000 emails/month.
            </Text>
          )}
        </CardContent>
      </Card>

      {/* Event triggers */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Bell className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Event Triggers</CardTitle>
              <CardDescription>
                Choose which events send notifications.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <label className="flex items-start gap-3 cursor-pointer p-3 rounded border-2 border-border hover:bg-accent transition">
            <input
              type="checkbox"
              checked={onNewMention}
              onChange={(e) => setOnNewMention(e.target.checked)}
              className="w-4 h-4 accent-primary mt-0.5"
            />
            <div>
              <Text as="span" className="text-sm font-medium">New mentions</Text>
              <Text as="p" className="text-xs text-muted-foreground">
                Get notified when new keyword matches are found across platforms.
              </Text>
            </div>
          </label>
          <label className="flex items-start gap-3 cursor-pointer p-3 rounded border-2 border-border hover:bg-accent transition">
            <input
              type="checkbox"
              checked={onHighIntent}
              onChange={(e) => setOnHighIntent(e.target.checked)}
              className="w-4 h-4 accent-primary mt-0.5"
            />
            <div>
              <Text as="span" className="text-sm font-medium">High-intent signals</Text>
              <Text as="p" className="text-xs text-muted-foreground">
                Alert when a mention scores 7+ on intent classification (buy signals, comparisons).
              </Text>
            </div>
          </label>
          <label className="flex items-start gap-3 cursor-pointer p-3 rounded border-2 border-border hover:bg-accent transition">
            <input
              type="checkbox"
              checked={onNewLead}
              onChange={(e) => setOnNewLead(e.target.checked)}
              className="w-4 h-4 accent-primary mt-0.5"
            />
            <div>
              <Text as="span" className="text-sm font-medium">New leads created</Text>
              <Text as="p" className="text-xs text-muted-foreground">
                Notify when mentions are converted to leads in the pipeline.
              </Text>
            </div>
          </label>
        </CardContent>
      </Card>

      {/* Save */}
      <div className="flex items-center gap-3">
        <Button
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending}
        >
          {saveMutation.isPending ? "Saving..." : "Save Alert Settings"}
        </Button>
        {testSuccess && (
          <Text as="p" className="text-sm text-green-600">
            {testSuccess} test notification sent!
          </Text>
        )}
        {saveMutation.isSuccess && !testSuccess && (
          <Text as="p" className="text-sm text-green-600">
            Settings saved.
          </Text>
        )}
      </div>
    </div>
  );
}

import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { listSessions, saveSession, deleteSession, testSession } from "@/lib/api";
import type { PlatformSession } from "@/lib/types";
import { Globe, Wifi, WifiOff, CheckCircle2, Circle, Trash2, FlaskConical } from "lucide-react";

export const Route = createFileRoute("/_dashboard/browser-sessions")({
  component: BrowserSessionsPage,
});

const platformMeta = {
  reddit: {
    label: "Reddit",
    color: "bg-orange-500",
    placeholder: "Paste your reddit_session cookie value...",
    usernamePlaceholder: "u/yourname",
    hint: 'Open reddit.com, DevTools → Application → Cookies → copy the value of "reddit_session"',
  },
  twitter: {
    label: "Twitter / X",
    color: "bg-sky-500",
    placeholder: "Paste your auth_token cookie value...",
    usernamePlaceholder: "@yourhandle",
    hint: 'Open x.com, DevTools → Application → Cookies → copy the value of "auth_token"',
  },
  linkedin: {
    label: "LinkedIn",
    color: "bg-blue-700",
    placeholder: "Paste your li_at cookie value...",
    usernamePlaceholder: "Your LinkedIn name",
    hint: 'Open linkedin.com, DevTools → Application → Cookies → copy the value of "li_at"',
  },
} as const;

function SessionCard({ session }: { session: PlatformSession }) {
  const queryClient = useQueryClient();
  const meta = platformMeta[session.platform];

  const [cookie, setCookie] = useState("");
  const [username, setUsername] = useState("");
  const [testResult, setTestResult] = useState<string | null>(null);

  const saveMutation = useMutation({
    mutationFn: () => saveSession(session.platform, { session_cookie: cookie, username }),
    onSuccess: () => {
      setCookie("");
      setUsername("");
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteSession(session.platform),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["sessions"] }),
  });

  const testMutation = useMutation({
    mutationFn: () => testSession(session.platform),
    onSuccess: (data) => setTestResult(data.message),
  });

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className={`w-10 h-10 rounded ${meta.color} border-2 border-border shadow-xs flex items-center justify-center`}>
            <Globe className="h-5 w-5 text-white" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <CardTitle>{meta.label}</CardTitle>
              {session.is_configured ? (
                <Badge variant="surface" size="sm" className="text-xs text-green-600 border-green-200">
                  <CheckCircle2 className="h-3 w-3 mr-1" />
                  {session.username ? `Connected as ${session.username}` : "Connected"}
                </Badge>
              ) : (
                <Badge variant="outline" size="sm" className="text-xs text-muted-foreground">
                  <Circle className="h-3 w-3 mr-1" />
                  Not configured
                </Badge>
              )}
            </div>
            <CardDescription className="flex items-center gap-1.5 mt-0.5">
              {session.is_pinchtab_online ? (
                <>
                  <Wifi className="h-3 w-3 text-green-500" />
                  <span className="text-green-600">Pinchtab online</span>
                </>
              ) : (
                <>
                  <WifiOff className="h-3 w-3 text-muted-foreground" />
                  <span>Pinchtab offline — set PINCHTAB_TOKEN to enable</span>
                </>
              )}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1.5">
          <Text as="p" className="text-sm font-medium">Session Cookie</Text>
          <Input
            type="password"
            placeholder={meta.placeholder}
            value={cookie}
            onChange={(e) => setCookie(e.target.value)}
          />
          <Text as="p" className="text-xs text-muted-foreground">{meta.hint}</Text>
        </div>

        <div className="space-y-1.5">
          <Text as="p" className="text-sm font-medium">Username (optional)</Text>
          <Input
            placeholder={meta.usernamePlaceholder}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
        </div>

        <Text as="p" className="text-xs text-muted-foreground">
          Cookies are encrypted at rest using AES-256-GCM.
        </Text>

        <div className="flex items-center gap-2 pt-1">
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={!cookie.trim() || saveMutation.isPending}
            size="sm"
          >
            Save
          </Button>
          {session.is_configured && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => deleteMutation.mutate()}
              disabled={deleteMutation.isPending}
              className="text-destructive hover:text-destructive"
            >
              <Trash2 className="h-3.5 w-3.5 mr-1" />
              Remove
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => { setTestResult(null); testMutation.mutate(); }}
            disabled={testMutation.isPending}
          >
            <FlaskConical className="h-3.5 w-3.5 mr-1" />
            Test
          </Button>
          {testResult && (
            <Text as="span" className="text-xs text-muted-foreground">{testResult}</Text>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function BrowserSessionsPage() {
  const { data: sessions, isLoading } = useQuery({
    queryKey: ["sessions"],
    queryFn: listSessions,
  });

  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <Text as="h2">Browser Sessions</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Paste your Reddit and Twitter session cookies to enable authenticated crawling via Pinchtab.
          This eliminates Reddit rate limits and enables Twitter monitoring at no API cost.
        </Text>
      </div>

      {isLoading ? (
        <Text as="p" className="text-sm text-muted-foreground">Loading...</Text>
      ) : (
        <div className="space-y-4">
          {(sessions ?? []).map((s) => (
            <SessionCard key={s.platform} session={s} />
          ))}
        </div>
      )}
    </div>
  );
}

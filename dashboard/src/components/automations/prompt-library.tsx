import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { BookMarked, Check, Copy } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Text } from "@/components/ui/text";
import { listSkills, type Skill } from "@/lib/automations-api";

// automations (P2 #17): prompt library — the docs/skills playbooks, served by the API.

function SkillRow({ skill }: { skill: Skill }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(skill.prompt);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setOpen(true); // clipboard blocked: show the prompt to copy by hand
    }
  };
  return (
    <div className="rounded-lg border border-border p-3 space-y-2">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Text as="p" className="text-sm font-medium">{skill.title}</Text>
          <Text as="p" className="text-xs text-muted-foreground">{skill.summary}</Text>
        </div>
        <div className="flex shrink-0 gap-1.5">
          <Button size="sm" variant="outline" onClick={copy}>
            {copied ? <Check className="h-3.5 w-3.5 mr-1" /> : <Copy className="h-3.5 w-3.5 mr-1" />}
            {copied ? "Copied" : "Copy prompt"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setOpen((v) => !v)}>
            {open ? "Hide" : "View"}
          </Button>
        </div>
      </div>
      <div className="flex flex-wrap gap-1">
        {skill.tools.map((t) => <Badge key={t} variant="outline" size="sm" className="font-mono">{t}</Badge>)}
        {skill.schedulable && <Badge variant="surface" size="sm">Schedulable: {skill.schedulable}</Badge>}
      </div>
      {open && (
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-3 text-xs leading-relaxed">
          {skill.content}
        </pre>
      )}
    </div>
  );
}

export function PromptLibrary() {
  const { data, isError } = useQuery({ queryKey: ["skills"], queryFn: listSkills, staleTime: Infinity });
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <BookMarked className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Prompt library</CardTitle>
            <CardDescription>
              Playbooks for Claude, ChatGPT or any MCP client connected with a workspace API key (Settings → API &amp; MCP).
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {isError && <Text as="p" className="text-sm text-destructive">Could not load the prompt library.</Text>}
        {!data && !isError && <Text as="p" className="text-sm text-muted-foreground">Loading...</Text>}
        {data?.map((s) => <SkillRow key={s.name} skill={s} />)}
      </CardContent>
    </Card>
  );
}

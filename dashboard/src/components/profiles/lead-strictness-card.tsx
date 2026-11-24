import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { SlidersHorizontal } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { getLeadStrictness, saveLeadStrictness } from "@/lib/api";
import { STRICTNESS_LEVELS, leadLineText } from "@/lib/lead-strictness";

const QUERY_KEY = ["lead-strictness"];

// One control for the lead line, so quality can be tuned without editing
// what the listener looks for.
export function LeadStrictnessCard() {
  const queryClient = useQueryClient();
  const { data, isError } = useQuery({ queryKey: QUERY_KEY, queryFn: getLeadStrictness });
  const save = useMutation({
    mutationFn: saveLeadStrictness,
    onSuccess: (next) => {
      queryClient.setQueryData(QUERY_KEY, next);
      queryClient.invalidateQueries(); // inbox lists, counts and badges all move with the line
    },
  });

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <SlidersHorizontal className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Lead strictness</CardTitle>
            <CardDescription>
              Stricter shows fewer leads that are more likely to buy but may miss some; looser catches more buyers along with more posts you will skip.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {data ? (
          <>
            <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Lead strictness">
              {STRICTNESS_LEVELS.map((l) => (
                <Button
                  key={l.value}
                  size="sm"
                  role="radio"
                  aria-checked={data.strictness === l.value}
                  variant={data.strictness === l.value ? "accent" : "outline"}
                  disabled={save.isPending}
                  onClick={() => data.strictness !== l.value && save.mutate(l.value)}
                >
                  {l.label}
                </Button>
              ))}
            </div>
            <Text as="p" className="text-xs text-muted-foreground">
              {leadLineText(data.min_score)}
            </Text>
            {save.error && <Text as="p" className="text-xs text-destructive">{save.error.message}</Text>}
          </>
        ) : (
          <Text as="p" className="text-sm text-muted-foreground">
            {isError ? "Could not load lead strictness." : "Loading..."}
          </Text>
        )}
      </CardContent>
    </Card>
  );
}

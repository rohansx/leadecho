import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { ArchetypeSelect } from "@/components/onboarding/archetype-select";
import type { BrandProfile } from "@/components/onboarding/types";
import { getBrandProfile, saveBrandProfile } from "@/lib/api";

const QUERY_KEY = ["brand-profile"];

function ProductTypeForm({ profile }: { profile: BrandProfile }) {
  const queryClient = useQueryClient();
  const [archetype, setArchetype] = useState(profile.archetype ?? "");
  const save = useMutation({
    // PUT replaces the whole profile: build it from a fresh read, not the
    // cached one, so only the archetype changes.
    mutationFn: async () => saveBrandProfile({ ...(await getBrandProfile()), archetype }),
    onSuccess: (next) => queryClient.setQueryData(QUERY_KEY, next),
  });
  return (
    <CardContent className="space-y-3">
      <ArchetypeSelect value={archetype} onChange={setArchetype} />
      <div className="flex items-center gap-3">
        <Button size="sm" variant="accent" disabled={save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "Saving..." : "Save"}
        </Button>
        {save.isSuccess && <Text as="span" className="text-xs text-muted-foreground">Saved. Posts scored from now on use this.</Text>}
        {save.error && <Text as="span" className="text-xs text-destructive">{save.error.message}</Text>}
      </div>
    </CardContent>
  );
}

/** Product type: picks the facts each lead shows (stack, team size, deadline ...). */
export function ProductTypeCard() {
  const { data, isError } = useQuery({ queryKey: QUERY_KEY, queryFn: getBrandProfile, retry: false });
  return (
    <Card>
      <CardHeader>
        <CardTitle>Product type</CardTitle>
        <CardDescription>
          Picks the facts a lead shows from what its post says, such as the tool they use now or their team size. Facts are only ever taken from the post.
        </CardDescription>
      </CardHeader>
      {data ? (
        <ProductTypeForm profile={data} />
      ) : (
        <CardContent>
          <Text as="p" className="text-sm text-muted-foreground">
            {isError ? "No product profile yet: finish onboarding first." : "Loading..."}
          </Text>
        </CardContent>
      )}
    </Card>
  );
}

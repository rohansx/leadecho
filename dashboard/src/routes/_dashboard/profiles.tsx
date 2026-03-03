import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  listProfiles,
  createProfile,
  updateProfile,
  deleteProfile,
} from "@/lib/api";
import type { MonitoringProfile } from "@/lib/types";
import { Plus, Trash2, Power, PowerOff, Target } from "lucide-react";

export const Route = createFileRoute("/_dashboard/profiles")({
  component: ProfilesPage,
});

function ProfilesPage() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [painPoints, setPainPoints] = useState("");

  const { data: profiles, isLoading } = useQuery({
    queryKey: ["profiles"],
    queryFn: listProfiles,
  });

  const addMutation = useMutation({
    mutationFn: () =>
      createProfile({
        name,
        description,
        pain_points: painPoints
          .split("\n")
          .map((l) => l.trim())
          .filter(Boolean),
      }),
    onSuccess: () => {
      setName("");
      setDescription("");
      setPainPoints("");
      queryClient.invalidateQueries({ queryKey: ["profiles"] });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, isActive }: { id: string; isActive: boolean }) =>
      updateProfile(id, { is_active: isActive }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["profiles"] }),
  });

  const deleteMutation = useMutation({
    mutationFn: deleteProfile,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["profiles"] }),
  });

  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <Text as="h2">Pain-Point Profiles</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Describe the problems your product solves. The system uses semantic matching to find people expressing these pain points.
        </Text>
      </div>

      {/* Add profile */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Plus className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Add Profile</CardTitle>
              <CardDescription>
                Define a customer pain point and the phrases people use to describe it.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-1.5">
            <Text as="p" className="text-sm font-medium">Name</Text>
            <Input
              placeholder="e.g. Slow CI/CD pipelines"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Text as="p" className="text-sm font-medium">Description</Text>
            <Input
              placeholder="Brief description of this pain point..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Text as="p" className="text-sm font-medium">Pain-Point Phrases</Text>
            <textarea
              className="w-full min-h-[100px] rounded border-2 border-border bg-background px-3 py-2 text-sm font-[family-name:var(--font-sans)] shadow-xs focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder={"my builds take forever\nCI is so slow\ntired of waiting for deploys\none per line..."}
              value={painPoints}
              onChange={(e) => setPainPoints(e.target.value)}
            />
            <Text as="p" className="text-xs text-muted-foreground">
              One phrase per line. These are embedded as vectors for semantic matching against incoming mentions.
            </Text>
          </div>

          <Button
            onClick={() => addMutation.mutate()}
            disabled={!name.trim() || addMutation.isPending}
          >
            <Plus className="h-3.5 w-3.5 mr-1" />
            Create Profile
          </Button>
        </CardContent>
      </Card>

      {/* Profile list */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Target className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Active Profiles</CardTitle>
              <CardDescription>
                {profiles?.length ?? 0} profile{(profiles?.length ?? 0) !== 1 ? "s" : ""} configured
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Text as="p" className="text-sm text-muted-foreground text-center py-8">
              Loading...
            </Text>
          ) : !profiles?.length ? (
            <div className="text-center py-8">
              <Target className="h-8 w-8 text-muted-foreground mx-auto mb-2 opacity-50" />
              <Text as="p" className="text-muted-foreground text-sm">
                No profiles yet. Add one above to start semantic monitoring.
              </Text>
            </div>
          ) : (
            <div className="space-y-2">
              {profiles.map((profile: MonitoringProfile) => (
                <div
                  key={profile.id}
                  className={`p-3 rounded border-2 border-border transition ${
                    profile.is_active ? "bg-card" : "bg-muted opacity-60"
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0">
                      <Text as="span" className="font-medium text-sm truncate">
                        {profile.name}
                      </Text>
                    </div>
                    <div className="flex items-center gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() =>
                          toggleMutation.mutate({
                            id: profile.id,
                            isActive: !profile.is_active,
                          })
                        }
                      >
                        {profile.is_active ? (
                          <Power className="h-3.5 w-3.5 text-green-600" />
                        ) : (
                          <PowerOff className="h-3.5 w-3.5 text-muted-foreground" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() => deleteMutation.mutate(profile.id)}
                      >
                        <Trash2 className="h-3.5 w-3.5 text-destructive" />
                      </Button>
                    </div>
                  </div>
                  {profile.description && (
                    <Text as="p" className="text-xs text-muted-foreground mt-1">
                      {profile.description}
                    </Text>
                  )}
                  {profile.pain_points.length > 0 && (
                    <div className="flex flex-wrap items-center gap-1.5 mt-2">
                      {profile.pain_points.map((phrase) => (
                        <Badge key={phrase} variant="surface" size="sm" className="text-xs">
                          {phrase}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

import { createFileRoute } from "@tanstack/react-router";
import { Text } from "@/components/ui/text";
import { Card, CardContent } from "@/components/ui/card";

export const Route = createFileRoute("/_dashboard/workflows")({
  component: WorkflowsPage,
});

function WorkflowsPage() {
  return (
    <div className="space-y-6">
      <Text as="h2">Workflows</Text>
      <Card className="w-full">
        <CardContent className="p-8 text-center">
          <Text as="p" className="text-muted-foreground">
            Visual workflow builder with triggers, conditions, and actions
            coming soon.
          </Text>
        </CardContent>
      </Card>
    </div>
  );
}

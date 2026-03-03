import { createFileRoute, Outlet } from "@tanstack/react-router";

export const Route = createFileRoute("/_auth")({
  component: AuthLayout,
});

function AuthLayout() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <span className="font-[family-name:var(--font-head)] text-3xl">
            LeadEcho
          </span>
        </div>
        <Outlet />
      </div>
    </div>
  );
}

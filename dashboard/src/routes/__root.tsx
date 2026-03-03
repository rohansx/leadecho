import { createRootRoute, Outlet } from "@tanstack/react-router";
import { ThemeProvider } from "@/providers/theme-provider";
import { QueryProvider } from "@/providers/query-provider";
import { AuthProvider } from "@/lib/auth";

export const Route = createRootRoute({
  component: RootComponent,
});

function RootComponent() {
  return (
    <ThemeProvider>
      <QueryProvider>
        <AuthProvider>
          <Outlet />
        </AuthProvider>
      </QueryProvider>
    </ThemeProvider>
  );
}

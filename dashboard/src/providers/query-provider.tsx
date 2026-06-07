import {
  QueryClient,
  QueryClientProvider,
  MutationCache,
} from "@tanstack/react-query";
import { useState, useEffect, type ReactNode } from "react";

// Minimal global error surface: failed mutations would otherwise fail silently
// (the UI just doesn't update). We push the API's clean error message into a
// transient toast so users actually see what went wrong.
let pushError: ((msg: string) => void) | null = null;

function ErrorToasts() {
  const [toasts, setToasts] = useState<{ id: number; msg: string }[]>([]);
  useEffect(() => {
    pushError = (msg: string) => {
      const id = Date.now() + Math.random();
      setToasts((t) => [...t, { id, msg }]);
      setTimeout(
        () => setToasts((t) => t.filter((x) => x.id !== id)),
        6000,
      );
    };
    return () => {
      pushError = null;
    };
  }, []);

  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
      {toasts.map((t) => (
        <div
          key={t.id}
          role="alert"
          className="bg-destructive text-destructive-foreground border-2 border-border rounded shadow-md px-4 py-2 text-sm font-[family-name:var(--font-sans)]"
        >
          {t.msg}
        </div>
      ))}
    </div>
  );
}

export function QueryProvider({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        mutationCache: new MutationCache({
          onError: (error) => {
            const msg =
              error instanceof Error && error.message
                ? error.message
                : "Something went wrong";
            pushError?.(msg);
          },
        }),
        defaultOptions: {
          queries: {
            staleTime: 60 * 1000,
            retry: 1,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      <ErrorToasts />
    </QueryClientProvider>
  );
}

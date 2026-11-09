import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import type { AuthProviders } from "@/lib/api";
import { ERROR_MESSAGES } from "./errors";

export function UrlError() {
  const params = new URLSearchParams(window.location.search);
  const code = params.get("error");
  if (!code && params.get("verified") === "1") {
    return (
      <Text as="p" role="status" className="text-sm text-green-600">
        Email verified — sign in.
      </Text>
    );
  }
  if (!code) return null;
  return (
    <Text as="p" role="alert" className="text-sm text-destructive">
      {ERROR_MESSAGES[code] ?? ERROR_MESSAGES.auth_failed}
    </Text>
  );
}

export function FormError({ message }: { message: string }) {
  if (!message) return null;
  return (
    <Text as="p" role="alert" className="text-xs text-destructive">
      {message}
    </Text>
  );
}

export function GoogleIcon() {
  return (
    <svg className="h-5 w-5 mr-2" viewBox="0 0 24 24" aria-hidden="true">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4" />
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853" />
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05" />
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335" />
    </svg>
  );
}

export function GitHubIcon() {
  return (
    <svg className="h-5 w-5 mr-2" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 .5a11.5 11.5 0 00-3.64 22.41c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.54-3.88-1.54-.53-1.33-1.28-1.69-1.28-1.69-1.05-.71.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.56-.29-5.25-1.28-5.25-5.68 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 015.77 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.83 1.19 3.09 0 4.41-2.7 5.38-5.27 5.67.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0012 .5z" />
    </svg>
  );
}

/** "Continue with Google/GitHub" buttons for the providers the server has enabled. */
export function OAuthButtons({ providers }: { providers?: AuthProviders }) {
  if (!providers?.google && !providers?.github) return null;
  return (
    <div className="space-y-2">
      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t border-border" />
        </div>
        <div className="relative flex justify-center text-xs">
          <span className="bg-card px-2 text-muted-foreground">or</span>
        </div>
      </div>
      {providers.google && (
        <Button asChild className="w-full" size="lg" variant="outline">
          <a href="/api/v1/auth/google">
            <GoogleIcon />
            Continue with Google
          </a>
        </Button>
      )}
      {providers.github && (
        <Button asChild className="w-full" size="lg" variant="outline">
          <a href="/api/v1/auth/github">
            <GitHubIcon />
            Continue with GitHub
          </a>
        </Button>
      )}
    </div>
  );
}

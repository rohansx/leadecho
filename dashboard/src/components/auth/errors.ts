/** Friendly copy for ?error= codes the backend redirects with. */
export const ERROR_MESSAGES: Record<string, string> = {
  invalid_state: "Your sign-in link expired. Please try again.",
  invalid_token: "That link is invalid or has expired. Request a new one.",
  oauth_denied: "Sign-in was cancelled.",
  email_unverified_provider: "That account has no verified email address. Verify it with the provider, or sign up with email.",
  account_exists: "An account with this email already exists. Sign in with your password.",
  auth_failed: "We couldn't sign you in. Please try again.",
};

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : "Something went wrong";
}

// chat (P2 #15): "Ask LeadEcho" API client.
import { request } from "./api";
import { streamSSE, type SSEHandler } from "@/components/onboarding/sse";

export interface ChatThread {
  id: string;
  title: string;
  updated_at: string;
}

export interface MentionCard {
  id: string;
  platform: string;
  url: string;
  title?: string;
  content: string;
  author?: string;
  relevance_score?: number;
  intent?: string;
}

export interface LeadCard {
  id: string;
  stage: string;
  mention_id?: string;
  contact_name?: string;
  username?: string;
  company?: string;
  platform?: string;
}

export type ToolCallStatus = "done" | "error" | "pending" | "running" | "cancelled";

export interface ToolCall {
  id: string;
  tool: string;
  args: Record<string, unknown>;
  status: ToolCallStatus;
  confirm?: boolean; // write tool gated on the user's confirm
  error?: string;
  cards?: { mentions?: MentionCard[]; leads?: LeadCard[] };
}

export interface ChatMessage {
  id: string;
  thread_id: string;
  role: "user" | "assistant";
  content: string;
  tool_calls: ToolCall[];
  created_at: string;
}

export const SUGGESTED_PROMPTS = [
  "Show me today's best leads",
  "Who is switching from a competitor this week?",
  "Why did this keyword make money?",
  "Write a reply for the top lead",
] as const;

export function listChatThreads(): Promise<{ data: ChatThread[] }> {
  return request("/chat/threads");
}

export function listChatMessages(threadId: string): Promise<{ data: ChatMessage[] }> {
  return request(`/chat/threads/${threadId}/messages`);
}

export async function deleteChatThread(threadId: string): Promise<void> {
  const res = await fetch(`/api/v1/chat/threads/${threadId}`, { method: "DELETE" });
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error || `Delete failed (${res.status})`);
  }
}

export function confirmChatAction(
  messageId: string,
  callId: string,
  approve: boolean,
): Promise<{ call: ToolCall; message: ChatMessage }> {
  return request(`/chat/messages/${messageId}/confirm`, {
    method: "POST",
    body: JSON.stringify({ call_id: callId, approve }),
  });
}

export function sendChat(
  message: string,
  threadId: string | undefined,
  onEvent: SSEHandler,
  signal?: AbortSignal,
  /** Scope the turn to one inbox item: the server adds what it knows about that lead. */
  mentionId?: string,
): Promise<void> {
  return streamSSE("/chat", { message, thread_id: threadId, mention_id: mentionId }, onEvent, signal);
}

/** The user's own customer list (free text); the lead assistant compares leads against it. */
export function getChatCustomers(): Promise<{ customers: string }> {
  return request("/chat/customers");
}

export function saveChatCustomers(customers: string): Promise<{ customers: string }> {
  return request("/chat/customers", { method: "PUT", body: JSON.stringify({ customers }) });
}

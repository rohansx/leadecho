export interface ExtensionSettings {
  apiKey: string;
  apiUrl: string;
}

const SETTINGS_KEY = "leadecho_settings";
const DAILY_COUNT_KEY = "leadecho_daily_count";

export async function getSettings(): Promise<ExtensionSettings> {
  const result = await chrome.storage.local.get(SETTINGS_KEY);
  return (result[SETTINGS_KEY] as ExtensionSettings | undefined) ?? { apiKey: "", apiUrl: "" };
}

export async function saveSettings(settings: ExtensionSettings): Promise<void> {
  await chrome.storage.local.set({ [SETTINGS_KEY]: settings });
}

export async function incrementDailyCount(n: number): Promise<number> {
  const today = new Date().toISOString().slice(0, 10);
  const result = await chrome.storage.local.get(DAILY_COUNT_KEY);
  const stored = result[DAILY_COUNT_KEY] as { date: string; count: number } | undefined;
  const existing = stored?.date === today ? stored.count : 0;
  const next = existing + n;
  await chrome.storage.local.set({ [DAILY_COUNT_KEY]: { date: today, count: next } });
  return next;
}

export async function getDailyCount(): Promise<number> {
  const today = new Date().toISOString().slice(0, 10);
  const result = await chrome.storage.local.get(DAILY_COUNT_KEY);
  const stored = result[DAILY_COUNT_KEY] as { date: string; count: number } | undefined;
  return stored?.date === today ? stored.count : 0;
}

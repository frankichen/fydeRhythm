export interface AiSettings {
  enabled: boolean;
  apiKey: string;
  model: "deepseek-v4-flash" | "deepseek-v4-pro";
  debounceMs: number;
  timeoutMs: number;
  contextChars: number;
  candidateLimit: number;
}

export const kAiSettingsKey = "aiSettings";

export const kDefaultAiSettings: AiSettings = {
  enabled: false,
  apiKey: "",
  model: "deepseek-v4-flash",
  debounceMs: 220,
  timeoutMs: 650,
  contextChars: 60,
  candidateLimit: 8,
};

function clamp(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.round(value)));
}

export function normalizeAiSettings(value?: Partial<AiSettings> | null): AiSettings {
  return {
    enabled: value?.enabled === true,
    apiKey: typeof value?.apiKey === "string" ? value.apiKey.trim() : "",
    model: value?.model === "deepseek-v4-pro" ? "deepseek-v4-pro" : "deepseek-v4-flash",
    debounceMs: clamp(value?.debounceMs, 100, 1000, kDefaultAiSettings.debounceMs),
    timeoutMs: clamp(value?.timeoutMs, 250, 3000, kDefaultAiSettings.timeoutMs),
    contextChars: clamp(value?.contextChars, 0, 100, kDefaultAiSettings.contextChars),
    candidateLimit: clamp(value?.candidateLimit, 2, 9, kDefaultAiSettings.candidateLimit),
  };
}

export async function loadAiSettings(): Promise<AiSettings> {
  const obj = await chrome.storage.local.get([kAiSettingsKey]) as { aiSettings?: Partial<AiSettings> };
  return normalizeAiSettings(obj.aiSettings);
}

export async function saveAiSettings(settings: AiSettings): Promise<void> {
  await chrome.storage.local.set({ [kAiSettingsKey]: normalizeAiSettings(settings) });
}

export const DEEPSEEK_STORAGE_KEY = "deepSeekSettings";
export const DEEPSEEK_API_URL = "https://api.deepseek.com/chat/completions";

export type DeepSeekModel = "deepseek-v4-flash" | "deepseek-v4-pro";

export interface DeepSeekSettings {
    enabled: boolean;
    apiKey: string;
    model: DeepSeekModel;
    timeoutMs: number;
    debounceMs: number;
    maxContextChars: number;
    minCandidates: number;
}

export const kDefaultDeepSeekSettings: DeepSeekSettings = {
    enabled: false,
    apiKey: "",
    model: "deepseek-v4-flash",
    timeoutMs: 650,
    debounceMs: 220,
    maxContextChars: 60,
    minCandidates: 2,
};

export function normalizeDeepSeekSettings(value: Partial<DeepSeekSettings> | null | undefined): DeepSeekSettings {
    const model: DeepSeekModel = value?.model === "deepseek-v4-pro"
        ? "deepseek-v4-pro"
        : "deepseek-v4-flash";

    return {
        enabled: value?.enabled === true,
        apiKey: typeof value?.apiKey === "string" ? value.apiKey.trim() : "",
        model,
        timeoutMs: clampNumber(value?.timeoutMs, 250, 3000, kDefaultDeepSeekSettings.timeoutMs),
        debounceMs: clampNumber(value?.debounceMs, 100, 1000, kDefaultDeepSeekSettings.debounceMs),
        maxContextChars: clampNumber(value?.maxContextChars, 0, 100, kDefaultDeepSeekSettings.maxContextChars),
        minCandidates: clampNumber(value?.minCandidates, 2, 9, kDefaultDeepSeekSettings.minCandidates),
    };
}

function clampNumber(value: unknown, min: number, max: number, fallback: number): number {
    if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
    return Math.min(max, Math.max(min, Math.round(value)));
}

export async function loadDeepSeekSettings(): Promise<DeepSeekSettings> {
    const stored = await chrome.storage.local.get([DEEPSEEK_STORAGE_KEY]) as {
        deepSeekSettings?: Partial<DeepSeekSettings>;
    };
    return normalizeDeepSeekSettings(stored.deepSeekSettings);
}

export async function saveDeepSeekSettings(settings: DeepSeekSettings): Promise<void> {
    await chrome.storage.local.set({
        [DEEPSEEK_STORAGE_KEY]: normalizeDeepSeekSettings(settings),
    });
}

export interface CandidateRankingRequest {
    context: string;
    preedit: string;
    candidates: string[];
}

interface DeepSeekChatResponse {
    choices?: Array<{
        message?: {
            content?: string | null;
        };
    }>;
}

export async function rerankCandidateOrder(
    request: CandidateRankingRequest,
    settingsInput?: DeepSeekSettings,
    externalSignal?: AbortSignal,
): Promise<number[] | null> {
    const settings = normalizeDeepSeekSettings(settingsInput ?? await loadDeepSeekSettings());
    if (!settings.enabled || !settings.apiKey || request.candidates.length < settings.minCandidates) {
        return null;
    }

    const controller = new AbortController();
    const abortFromOutside = () => controller.abort();
    externalSignal?.addEventListener("abort", abortFromOutside, { once: true });
    const timeout = setTimeout(() => controller.abort(), settings.timeoutMs);

    try {
        const response = await fetch(DEEPSEEK_API_URL, {
            method: "POST",
            headers: {
                "Authorization": `Bearer ${settings.apiKey}`,
                "Content-Type": "application/json",
            },
            signal: controller.signal,
            body: JSON.stringify({
                model: settings.model,
                thinking: { type: "disabled" },
                stream: false,
                temperature: 0,
                max_tokens: 96,
                response_format: { type: "json_object" },
                messages: [
                    {
                        role: "system",
                        content: "你是中文输入法候选排序器。根据光标前文、当前编码和候选词，返回最符合语境的候选顺序。只能输出 JSON，例如 {\"order\":[2,0,1]}。order 必须只包含候选下标，不得创造新词，不要解释。",
                    },
                    {
                        role: "user",
                        content: JSON.stringify({
                            context: request.context,
                            preedit: request.preedit,
                            candidates: request.candidates,
                        }),
                    },
                ],
            }),
        });

        if (!response.ok) {
            const detail = (await response.text()).slice(0, 300);
            throw new Error(`DeepSeek HTTP ${response.status}: ${detail || response.statusText}`);
        }

        const payload = await response.json() as DeepSeekChatResponse;
        const content = payload.choices?.[0]?.message?.content;
        if (!content) throw new Error("DeepSeek returned an empty response");

        const parsed = parseJsonObject(content) as { order?: unknown };
        return normalizeOrder(parsed.order, request.candidates.length);
    } finally {
        clearTimeout(timeout);
        externalSignal?.removeEventListener("abort", abortFromOutside);
    }
}

function parseJsonObject(content: string): unknown {
    const trimmed = content.trim();
    try {
        return JSON.parse(trimmed);
    } catch {
        const match = trimmed.match(/\{[\s\S]*\}/);
        if (!match) throw new Error("DeepSeek response was not valid JSON");
        return JSON.parse(match[0]);
    }
}

function normalizeOrder(value: unknown, candidateCount: number): number[] | null {
    if (!Array.isArray(value)) return null;

    const seen = new Set<number>();
    const order: number[] = [];
    for (const item of value) {
        if (typeof item !== "number" || !Number.isInteger(item)) continue;
        if (item < 0 || item >= candidateCount || seen.has(item)) continue;
        seen.add(item);
        order.push(item);
    }
    if (order.length === 0) return null;

    for (let i = 0; i < candidateCount; i++) {
        if (!seen.has(i)) order.push(i);
    }
    return order;
}

export async function testDeepSeekSettings(settings: DeepSeekSettings): Promise<number[]> {
    const enabledSettings = normalizeDeepSeekSettings({ ...settings, enabled: true });
    const result = await rerankCandidateOrder({
        context: "这个功能修改完成后，我们先进行",
        preedit: "cs",
        candidates: ["测试", "尝试", "超时", "重试"],
    }, enabledSettings);
    if (!result) throw new Error("DeepSeek did not return a candidate order");
    return result;
}

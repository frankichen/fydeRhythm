import type { AiSettings } from "./ai-settings";

const kDeepSeekEndpoint = "https://api.deepseek.com/chat/completions";

interface DeepSeekResponse {
  choices?: Array<{ message?: { content?: string | null } }>;
}

export function normalizeCandidateOrder(value: unknown, count: number): number[] | null {
  if (!value || typeof value !== "object") return null;
  const raw = (value as { order?: unknown }).order;
  if (!Array.isArray(raw)) return null;

  const seen = new Set<number>();
  const result: number[] = [];
  for (const item of raw) {
    if (!Number.isInteger(item)) continue;
    const index = item as number;
    if (index < 0 || index >= count || seen.has(index)) continue;
    seen.add(index);
    result.push(index);
  }
  if (result.length === 0) return null;
  for (let index = 0; index < count; index++) {
    if (!seen.has(index)) result.push(index);
  }
  return result;
}

function parseJsonObject(content: string): unknown {
  const text = content.trim();
  try {
    return JSON.parse(text);
  } catch {
    const match = text.match(/\{[\s\S]*\}/);
    if (!match) return null;
    try {
      return JSON.parse(match[0]);
    } catch {
      return null;
    }
  }
}

export async function rankCandidatesWithDeepSeek(
  settings: AiSettings,
  context: string,
  preedit: string,
  candidates: string[],
  signal: AbortSignal,
): Promise<number[] | null> {
  if (!settings.enabled || !settings.apiKey || candidates.length < 2) return null;

  const response = await fetch(kDeepSeekEndpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${settings.apiKey}`,
    },
    signal,
    body: JSON.stringify({
      model: settings.model,
      thinking: { type: "disabled" },
      temperature: 0,
      max_tokens: 96,
      response_format: { type: "json_object" },
      messages: [
        {
          role: "system",
          content: "你是中文输入法候选排序器。根据光标前文、当前编码和候选词，只调整现有候选顺序。只输出 JSON，例如 {\"order\":[2,0,1]}。order 中只能出现候选下标，不得创造、删除或改写候选，不要解释。",
        },
        {
          role: "user",
          content: JSON.stringify({ context, preedit, candidates }),
        },
      ],
    }),
  });

  if (!response.ok) {
    const detail = (await response.text()).slice(0, 240);
    throw new Error(`DeepSeek HTTP ${response.status}: ${detail || response.statusText}`);
  }

  const payload = await response.json() as DeepSeekResponse;
  const content = payload.choices?.[0]?.message?.content;
  if (!content) return null;
  return normalizeCandidateOrder(parseJsonObject(content), candidates.length);
}

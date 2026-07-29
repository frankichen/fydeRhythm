import {
  kAiSettingsKey,
  kDefaultAiSettings,
  loadAiSettings,
  normalizeAiSettings,
  type AiSettings,
} from "@/lib/ai-settings";
import { rankCandidatesWithDeepSeek } from "@/lib/deepseek-ranker";

interface CandidateItem {
  candidate: string;
  id: number;
  label?: string;
  annotation?: string;
  usage?: { title: string; body: string };
}

export class AiCandidateCoordinator {
  private settings: AiSettings = kDefaultAiSettings;
  private context: chrome.input.ime.InputContext | null = null;
  private surroundingText = "";
  private preedit = "";
  private timer: ReturnType<typeof setTimeout> | null = null;
  private abortController: AbortController | null = null;
  private requestSerial = 0;
  private originalSetCandidates: typeof chrome.input.ime.setCandidates | null = null;
  private internalUpdate = false;
  private activeOrder: number[] | null = null;
  private activeContextId: number | null = null;

  async initialize(): Promise<void> {
    this.settings = await loadAiSettings();
    this.installSetCandidatesInterceptor();
    chrome.storage.onChanged.addListener((changes, areaName) => {
      if (areaName !== "local" || !changes[kAiSettingsKey]) return;
      this.settings = normalizeAiSettings(changes[kAiSettingsKey].newValue as Partial<AiSettings> | undefined);
      this.cancelPending();
      this.clearOrder();
    });
  }

  setInputContext(context: chrome.input.ime.InputContext | null): void {
    this.context = context;
    this.surroundingText = "";
    this.preedit = "";
    this.cancelPending();
    this.clearOrder();
  }

  setSurroundingText(info: { text: string; focus: number; offset?: number }): void {
    const relativeFocus = Math.max(0, info.focus - (info.offset ?? 0));
    this.surroundingText = info.text.slice(0, relativeFocus).slice(-this.settings.contextChars);
  }

  setPreedit(preedit: string): void {
    this.preedit = preedit;
  }

  getMappedCandidate(keyData: chrome.input.ime.KeyboardEvent): number | null {
    if (!this.activeOrder || this.activeContextId !== this.context?.contextID) return null;
    if (keyData.type === "keyup" || keyData.ctrlKey || keyData.altKey || keyData.shiftKey) return null;

    if (/^[1-9]$/.test(keyData.key)) {
      return this.activeOrder[Number(keyData.key) - 1] ?? null;
    }
    if (keyData.code === "Space" || keyData.code === "Enter") {
      return this.activeOrder[0] ?? null;
    }
    if (["ArrowUp", "ArrowDown", "PageUp", "PageDown"].includes(keyData.code)) {
      this.clearOrder();
    }
    return null;
  }

  clearOrder(): void {
    this.activeOrder = null;
    this.activeContextId = null;
  }

  private isPrivateContext(): boolean {
    if (!this.context) return true;
    const type = String(this.context.type ?? "null").toLowerCase();
    if (["password", "number", "tel", "url", "email", "null"].includes(type)) return true;
    return this.context.shouldDoLearning === false;
  }

  private installSetCandidatesInterceptor(): void {
    if (this.originalSetCandidates) return;
    this.originalSetCandidates = chrome.input.ime.setCandidates.bind(chrome.input.ime);
    const coordinator = this;
    const inputIme = chrome.input.ime as typeof chrome.input.ime & {
      setCandidates: typeof chrome.input.ime.setCandidates;
    };

    inputIme.setCandidates = function (parameters, callback) {
      const original = coordinator.originalSetCandidates;
      if (!original) return;
      original(parameters, callback);
      if (!coordinator.internalUpdate) {
        coordinator.schedule(parameters.contextID, parameters.candidates as CandidateItem[]);
      }
    };
  }

  private schedule(contextId: number, candidates: CandidateItem[]): void {
    this.cancelPending();
    this.clearOrder();
    if (!this.settings.enabled || !this.settings.apiKey || this.isPrivateContext()) return;

    const limited = candidates.slice(0, this.settings.candidateLimit);
    if (limited.length < 2) return;
    const serial = ++this.requestSerial;
    const snapshot = limited.map((item) => item.candidate);
    const preedit = this.preedit;

    this.timer = setTimeout(() => {
      void this.runRank(serial, contextId, limited, snapshot, preedit);
    }, this.settings.debounceMs);
  }

  private async runRank(
    serial: number,
    contextId: number,
    candidates: CandidateItem[],
    snapshot: string[],
    preedit: string,
  ): Promise<void> {
    const original = this.originalSetCandidates;
    if (!original || this.context?.contextID !== contextId) return;

    this.abortController = new AbortController();
    const timeout = setTimeout(() => this.abortController?.abort(), this.settings.timeoutMs);
    try {
      const order = await rankCandidatesWithDeepSeek(
        this.settings,
        this.surroundingText,
        preedit,
        snapshot,
        this.abortController.signal,
      );
      if (!order || serial !== this.requestSerial || this.context?.contextID !== contextId) return;
      if (order.every((value, index) => value === index)) return;

      const reordered = order.map((sourceIndex, displayIndex) => ({
        ...candidates[sourceIndex],
        label: String(displayIndex + 1),
      }));
      this.activeOrder = order.map((sourceIndex) => candidates[sourceIndex].id);
      this.activeContextId = contextId;
      this.internalUpdate = true;
      original({ contextID: contextId, candidates: reordered }, () => {
        chrome.runtime.lastError;
      });
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        console.warn("AI candidate ranking failed:", error);
      }
    } finally {
      clearTimeout(timeout);
      this.internalUpdate = false;
      this.abortController = null;
    }
  }

  private cancelPending(): void {
    this.requestSerial++;
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.abortController?.abort();
    this.abortController = null;
  }
}

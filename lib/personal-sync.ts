import { openDB, type DBSchema, type IDBPDatabase } from "idb";

export const PERSONAL_SYNC_STORAGE_KEY = "personalSyncSettings";
export const PERSONAL_SYNC_STATUS_KEY = "personalSyncStatus";
export const PERSONAL_SYNC_ALARM_PULL = "personal-sync-pull";
export const PERSONAL_SYNC_ALARM_PUSH = "personal-sync-push";
export const PERSONAL_SYNC_DEFAULT_SERVER = "https://shulufa.555044.xyz";

const DB_NAME = "fyderhythm-personal-sync";
const DB_VERSION = 1;
const META_CURSOR = "cursor";
const META_BOOTSTRAPPED = "bootstrapped";
const MAX_PULL_PAGES = 20;
const LOCAL_CHANGE_RETENTION_MS = 30 * 24 * 60 * 60 * 1000;

export type PersonalSyncMode = "full" | "download-only" | "manual";
export type SyncEntity = "lexicon" | "candidate_usage" | "correction" | "ai_feedback" | "setting";
export type SyncOperation = "upsert" | "delete" | "increment";

export interface PersonalSyncSettings {
    enabled: boolean;
    serverUrl: string;
    apiToken: string;
    deviceId: string;
    deviceName: string;
    mode: PersonalSyncMode;
    uploadIntervalMs: number;
    pullIntervalMs: number;
    idleFlushMs: number;
    batchSize: number;
}

export interface PersonalSyncStatus {
    state: "disabled" | "idle" | "syncing" | "error";
    lastSyncAt: number;
    lastUploadAt: number;
    lastPullAt: number;
    pendingCount: number;
    cursor: number;
    error: string;
}

export interface LexiconEntry {
    id: string;
    phrase: string;
    code: string;
    shortcut: string;
    weight: number;
    category: string;
    notes: string;
    deleted: boolean;
    client_updated_at: number;
    source_device_id: string;
    server_updated_at: number;
    version: number;
}

export interface CandidateUsage {
    key: string;
    code: string;
    candidate: string;
    count: number;
    last_used_at: number;
}

export interface CorrectionUsage {
    key: string;
    original: string;
    corrected: string;
    count: number;
    last_used_at: number;
}

export interface AIFeedbackUsage {
    key: string;
    feature: string;
    context_hash: string;
    accepted_count: number;
    rejected_count: number;
    last_used_at: number;
}

export interface RemoteSetting {
    key: string;
    value: unknown;
    deleted: boolean;
    client_updated_at: number;
    source_device_id: string;
    server_updated_at: number;
    version: number;
}

export interface SyncChange {
    change_id: string;
    entity: SyncEntity;
    operation: SyncOperation;
    client_updated_at: number;
    payload: Record<string, unknown>;
}

interface PendingChange extends SyncChange {
    created_at: number;
}

interface LocalChangeMarker {
    change_id: string;
    created_at: number;
}

interface MetaRecord {
    key: string;
    value: unknown;
}

interface PersonalSyncDB extends DBSchema {
    meta: {
        key: string;
        value: MetaRecord;
    };
    pending: {
        key: string;
        value: PendingChange;
        indexes: { "by-created": number };
    };
    localChanges: {
        key: string;
        value: LocalChangeMarker;
        indexes: { "by-created": number };
    };
    lexicon: {
        key: string;
        value: LexiconEntry;
        indexes: { "by-code": string; "by-shortcut": string; "by-phrase": string };
    };
    candidateUsage: {
        key: string;
        value: CandidateUsage;
    };
    corrections: {
        key: string;
        value: CorrectionUsage;
    };
    aiFeedback: {
        key: string;
        value: AIFeedbackUsage;
    };
    remoteSettings: {
        key: string;
        value: RemoteSetting;
    };
}

interface PushResult {
    cursor: number;
    applied: number;
    duplicates: number;
    conflicts: number;
    rejected?: Array<{ change_id: string; reason: string }>;
}

interface LogChange extends SyncChange {
    seq: number;
    entity_key: string;
    source_device_id: string;
    created_at: number;
}

interface PullResult {
    cursor: number;
    has_more: boolean;
    changes: LogChange[];
}

interface SnapshotResult {
    cursor: number;
    generated_at: number;
    lexicon: LexiconEntry[];
    candidate_usage: Array<Omit<CandidateUsage, "key">>;
    corrections: Array<Omit<CorrectionUsage, "key">>;
    ai_feedback: Array<Omit<AIFeedbackUsage, "key">>;
    settings: RemoteSetting[];
}

export const kDefaultPersonalSyncSettings: PersonalSyncSettings = {
    enabled: false,
    serverUrl: PERSONAL_SYNC_DEFAULT_SERVER,
    apiToken: "",
    deviceId: "",
    deviceName: "Chromebook",
    mode: "full",
    uploadIntervalMs: 30_000,
    pullIntervalMs: 5 * 60_000,
    idleFlushMs: 5_000,
    batchSize: 50,
};

export const kDefaultPersonalSyncStatus: PersonalSyncStatus = {
    state: "disabled",
    lastSyncAt: 0,
    lastUploadAt: 0,
    lastPullAt: 0,
    pendingCount: 0,
    cursor: 0,
    error: "",
};

let dbPromise: Promise<IDBPDatabase<PersonalSyncDB>> | null = null;
let singletonManager: PersonalSyncManager | null = null;

export function normalizeServerUrl(value: unknown): string {
    const raw = typeof value === "string" ? value.trim() : "";
    if (!raw) return PERSONAL_SYNC_DEFAULT_SERVER;
    try {
        const url = new URL(raw);
        if (url.protocol !== "https:" && url.hostname !== "localhost" && url.hostname !== "127.0.0.1") {
            return PERSONAL_SYNC_DEFAULT_SERVER;
        }
        url.pathname = url.pathname.replace(/\/+$/, "");
        url.search = "";
        url.hash = "";
        return url.toString().replace(/\/$/, "");
    } catch {
        return PERSONAL_SYNC_DEFAULT_SERVER;
    }
}

export function normalizePersonalSyncSettings(value: Partial<PersonalSyncSettings> | null | undefined): PersonalSyncSettings {
    const mode: PersonalSyncMode = value?.mode === "download-only" || value?.mode === "manual"
        ? value.mode
        : "full";
    return {
        enabled: value?.enabled === true,
        serverUrl: normalizeServerUrl(value?.serverUrl),
        apiToken: typeof value?.apiToken === "string" ? value.apiToken.trim() : "",
        deviceId: typeof value?.deviceId === "string" ? value.deviceId.trim().slice(0, 128) : "",
        deviceName: typeof value?.deviceName === "string" && value.deviceName.trim()
            ? value.deviceName.trim().slice(0, 128)
            : kDefaultPersonalSyncSettings.deviceName,
        mode,
        uploadIntervalMs: clampNumber(value?.uploadIntervalMs, 10_000, 10 * 60_000, kDefaultPersonalSyncSettings.uploadIntervalMs),
        pullIntervalMs: clampNumber(value?.pullIntervalMs, 60_000, 60 * 60_000, kDefaultPersonalSyncSettings.pullIntervalMs),
        idleFlushMs: clampNumber(value?.idleFlushMs, 1_000, 60_000, kDefaultPersonalSyncSettings.idleFlushMs),
        batchSize: clampNumber(value?.batchSize, 10, 500, kDefaultPersonalSyncSettings.batchSize),
    };
}

function clampNumber(value: unknown, min: number, max: number, fallback: number): number {
    if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
    return Math.min(max, Math.max(min, Math.round(value)));
}

function newId(): string {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
        return crypto.randomUUID();
    }
    const random = Math.random().toString(16).slice(2);
    return `${Date.now().toString(16)}-${random}`;
}

function createDeviceId(): string {
    return `chromeos-${newId()}`.slice(0, 128);
}

export function candidateUsageKey(code: string, candidate: string): string {
    return `${code.trim()}\u0000${candidate.trim()}`;
}

export function correctionUsageKey(original: string, corrected: string): string {
    return `${original.trim()}\u0000${corrected.trim()}`;
}

export function aiFeedbackKey(feature: string, contextHash: string): string {
    return `${feature.trim()}\u0000${contextHash.trim()}`;
}

async function openPersonalSyncDB(): Promise<IDBPDatabase<PersonalSyncDB>> {
    if (!dbPromise) {
        dbPromise = openDB<PersonalSyncDB>(DB_NAME, DB_VERSION, {
            upgrade(db) {
                db.createObjectStore("meta", { keyPath: "key" });
                const pending = db.createObjectStore("pending", { keyPath: "change_id" });
                pending.createIndex("by-created", "created_at");
                const localChanges = db.createObjectStore("localChanges", { keyPath: "change_id" });
                localChanges.createIndex("by-created", "created_at");
                const lexicon = db.createObjectStore("lexicon", { keyPath: "id" });
                lexicon.createIndex("by-code", "code");
                lexicon.createIndex("by-shortcut", "shortcut");
                lexicon.createIndex("by-phrase", "phrase");
                db.createObjectStore("candidateUsage", { keyPath: "key" });
                db.createObjectStore("corrections", { keyPath: "key" });
                db.createObjectStore("aiFeedback", { keyPath: "key" });
                db.createObjectStore("remoteSettings", { keyPath: "key" });
            },
        });
    }
    return dbPromise;
}

async function getMeta<T>(key: string, fallback: T): Promise<T> {
    const db = await openPersonalSyncDB();
    const record = await db.get("meta", key);
    return (record?.value as T | undefined) ?? fallback;
}

async function setMeta(key: string, value: unknown): Promise<void> {
    const db = await openPersonalSyncDB();
    await db.put("meta", { key, value });
}

export async function loadPersonalSyncSettings(): Promise<PersonalSyncSettings> {
    const stored = await chrome.storage.local.get([PERSONAL_SYNC_STORAGE_KEY]) as {
        personalSyncSettings?: Partial<PersonalSyncSettings>;
    };
    const settings = normalizePersonalSyncSettings(stored.personalSyncSettings);
    if (!settings.deviceId) {
        settings.deviceId = createDeviceId();
        await chrome.storage.local.set({ [PERSONAL_SYNC_STORAGE_KEY]: settings });
    }
    return settings;
}

export async function savePersonalSyncSettings(input: PersonalSyncSettings): Promise<PersonalSyncSettings> {
    const settings = normalizePersonalSyncSettings(input);
    if (!settings.deviceId) settings.deviceId = createDeviceId();
    await chrome.storage.local.set({ [PERSONAL_SYNC_STORAGE_KEY]: settings });
    return settings;
}

export async function loadPersonalSyncStatus(): Promise<PersonalSyncStatus> {
    const stored = await chrome.storage.local.get([PERSONAL_SYNC_STATUS_KEY]) as {
        personalSyncStatus?: Partial<PersonalSyncStatus>;
    };
    return { ...kDefaultPersonalSyncStatus, ...stored.personalSyncStatus };
}

async function savePersonalSyncStatus(patch: Partial<PersonalSyncStatus>): Promise<PersonalSyncStatus> {
    const current = await loadPersonalSyncStatus();
    const next = { ...current, ...patch };
    await chrome.storage.local.set({ [PERSONAL_SYNC_STATUS_KEY]: next });
    return next;
}

function canUpload(settings: PersonalSyncSettings): boolean {
    return settings.enabled && settings.mode !== "download-only" && Boolean(settings.apiToken);
}

function canContactServer(settings: PersonalSyncSettings): boolean {
    return settings.enabled && Boolean(settings.serverUrl) && Boolean(settings.apiToken);
}

async function apiRequest<T>(settings: PersonalSyncSettings, path: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(`${settings.serverUrl}${path}`, {
        ...init,
        cache: "no-store",
        headers: {
            "Authorization": `Bearer ${settings.apiToken}`,
            "Content-Type": "application/json",
            "Cache-Control": "no-store",
            ...(init.headers ?? {}),
        },
    });
    if (!response.ok) {
        const detail = (await response.text()).slice(0, 300);
        throw new Error(`同步服务器 HTTP ${response.status}: ${detail || response.statusText}`);
    }
    return await response.json() as T;
}

async function enqueueChange(change: SyncChange): Promise<void> {
    const db = await openPersonalSyncDB();
    const createdAt = Date.now();
    const tx = db.transaction(["pending", "localChanges"], "readwrite");
    await tx.objectStore("pending").put({ ...change, created_at: createdAt });
    await tx.objectStore("localChanges").put({ change_id: change.change_id, created_at: createdAt });
    await tx.done;
    await updatePendingStatus();
    getPersonalSyncManager().scheduleIdleFlush();
}

async function updatePendingStatus(): Promise<number> {
    const db = await openPersonalSyncDB();
    const pendingCount = await db.count("pending");
    const cursor = await getMeta(META_CURSOR, 0);
    await savePersonalSyncStatus({ pendingCount, cursor });
    return pendingCount;
}

export async function recordCandidateUsage(codeInput: string, candidateInput: string): Promise<void> {
    const code = codeInput.trim().slice(0, 128);
    const candidate = candidateInput.trim().slice(0, 256);
    if (!code || !candidate) return;
    const now = Date.now();
    const key = candidateUsageKey(code, candidate);
    const db = await openPersonalSyncDB();
    const current = await db.get("candidateUsage", key);
    await db.put("candidateUsage", {
        key,
        code,
        candidate,
        count: (current?.count ?? 0) + 1,
        last_used_at: now,
    });
    const settings = await loadPersonalSyncSettings();
    if (canUpload(settings)) {
        await enqueueChange({
            change_id: newId(),
            entity: "candidate_usage",
            operation: "increment",
            client_updated_at: now,
            payload: { code, candidate, delta: 1, last_used_at: now },
        });
    }
}

export async function recordCorrection(originalInput: string, correctedInput: string): Promise<void> {
    const original = originalInput.trim().slice(0, 512);
    const corrected = correctedInput.trim().slice(0, 512);
    if (!original || !corrected || original === corrected) return;
    const now = Date.now();
    const key = correctionUsageKey(original, corrected);
    const db = await openPersonalSyncDB();
    const current = await db.get("corrections", key);
    await db.put("corrections", {
        key,
        original,
        corrected,
        count: (current?.count ?? 0) + 1,
        last_used_at: now,
    });
    const settings = await loadPersonalSyncSettings();
    if (canUpload(settings)) {
        await enqueueChange({
            change_id: newId(),
            entity: "correction",
            operation: "increment",
            client_updated_at: now,
            payload: { original, corrected, delta: 1, last_used_at: now },
        });
    }
}

export async function recordAIFeedback(featureInput: string, accepted: boolean, contextHash = ""): Promise<void> {
    const feature = featureInput.trim().slice(0, 64);
    const hash = contextHash.trim().slice(0, 128);
    if (!feature) return;
    const now = Date.now();
    const key = aiFeedbackKey(feature, hash);
    const db = await openPersonalSyncDB();
    const current = await db.get("aiFeedback", key);
    await db.put("aiFeedback", {
        key,
        feature,
        context_hash: hash,
        accepted_count: (current?.accepted_count ?? 0) + (accepted ? 1 : 0),
        rejected_count: (current?.rejected_count ?? 0) + (accepted ? 0 : 1),
        last_used_at: now,
    });
    const settings = await loadPersonalSyncSettings();
    if (canUpload(settings)) {
        await enqueueChange({
            change_id: newId(),
            entity: "ai_feedback",
            operation: "increment",
            client_updated_at: now,
            payload: {
                feature,
                context_hash: hash,
                accepted_delta: accepted ? 1 : 0,
                rejected_delta: accepted ? 0 : 1,
                last_used_at: now,
            },
        });
    }
}

export interface LexiconInput {
    id?: string;
    phrase: string;
    code?: string;
    shortcut?: string;
    weight?: number;
    category?: string;
    notes?: string;
}

export async function upsertLocalLexicon(input: LexiconInput): Promise<LexiconEntry> {
    const phrase = input.phrase.trim().slice(0, 256);
    if (!phrase) throw new Error("词语不能为空");
    const now = Date.now();
    const entry: LexiconEntry = {
        id: input.id?.trim() || newId(),
        phrase,
        code: (input.code ?? "").trim().slice(0, 128),
        shortcut: (input.shortcut ?? "").trim().slice(0, 128),
        weight: clampNumber(input.weight, -100000, 100000, 100),
        category: (input.category ?? "").trim().slice(0, 128),
        notes: (input.notes ?? "").trim().slice(0, 512),
        deleted: false,
        client_updated_at: now,
        source_device_id: "local",
        server_updated_at: 0,
        version: 1,
    };
    const db = await openPersonalSyncDB();
    const previous = await db.get("lexicon", entry.id);
    if (previous) entry.version = previous.version + 1;
    await db.put("lexicon", entry);
    const settings = await loadPersonalSyncSettings();
    if (canUpload(settings)) {
        await enqueueChange({
            change_id: newId(),
            entity: "lexicon",
            operation: "upsert",
            client_updated_at: now,
            payload: {
                id: entry.id,
                phrase: entry.phrase,
                code: entry.code,
                shortcut: entry.shortcut,
                weight: entry.weight,
                category: entry.category,
                notes: entry.notes,
            },
        });
    }
    return entry;
}

export async function deleteLocalLexicon(idInput: string): Promise<void> {
    const id = idInput.trim();
    if (!id) return;
    const db = await openPersonalSyncDB();
    const current = await db.get("lexicon", id);
    if (!current) return;
    const now = Date.now();
    await db.put("lexicon", {
        ...current,
        deleted: true,
        client_updated_at: now,
        source_device_id: "local",
        version: current.version + 1,
    });
    const settings = await loadPersonalSyncSettings();
    if (canUpload(settings)) {
        await enqueueChange({
            change_id: newId(),
            entity: "lexicon",
            operation: "delete",
            client_updated_at: now,
            payload: { id },
        });
    }
}

export async function listLocalLexicon(includeDeleted = false): Promise<LexiconEntry[]> {
    const db = await openPersonalSyncDB();
    const all = await db.getAll("lexicon");
    return all
        .filter((entry) => includeDeleted || !entry.deleted)
        .sort((a, b) => b.weight - a.weight || b.client_updated_at - a.client_updated_at || a.phrase.localeCompare(b.phrase));
}

export async function findLexiconSuggestions(inputCode: string, limit = 3): Promise<LexiconEntry[]> {
    const query = inputCode.trim();
    if (!query) return [];
    const db = await openPersonalSyncDB();
    const [byCode, byShortcut] = await Promise.all([
        db.getAllFromIndex("lexicon", "by-code", query),
        db.getAllFromIndex("lexicon", "by-shortcut", query),
    ]);
    const unique = new Map<string, LexiconEntry>();
    for (const entry of [...byShortcut, ...byCode]) {
        if (!entry.deleted) unique.set(entry.id, entry);
    }
    return [...unique.values()]
        .sort((a, b) => b.weight - a.weight || b.client_updated_at - a.client_updated_at)
        .slice(0, Math.max(1, limit));
}

async function applySnapshot(snapshot: SnapshotResult): Promise<void> {
    const db = await openPersonalSyncDB();
    const tx = db.transaction([
        "meta", "lexicon", "candidateUsage", "corrections", "aiFeedback", "remoteSettings",
    ], "readwrite");
    await Promise.all([
        tx.objectStore("lexicon").clear(),
        tx.objectStore("candidateUsage").clear(),
        tx.objectStore("corrections").clear(),
        tx.objectStore("aiFeedback").clear(),
        tx.objectStore("remoteSettings").clear(),
    ]);
    for (const entry of snapshot.lexicon ?? []) await tx.objectStore("lexicon").put(entry);
    for (const item of snapshot.candidate_usage ?? []) {
        await tx.objectStore("candidateUsage").put({ ...item, key: candidateUsageKey(item.code, item.candidate) });
    }
    for (const item of snapshot.corrections ?? []) {
        await tx.objectStore("corrections").put({ ...item, key: correctionUsageKey(item.original, item.corrected) });
    }
    for (const item of snapshot.ai_feedback ?? []) {
        await tx.objectStore("aiFeedback").put({ ...item, key: aiFeedbackKey(item.feature, item.context_hash) });
    }
    for (const item of snapshot.settings ?? []) await tx.objectStore("remoteSettings").put(item);
    await tx.objectStore("meta").put({ key: META_CURSOR, value: snapshot.cursor ?? 0 });
    await tx.objectStore("meta").put({ key: META_BOOTSTRAPPED, value: true });
    await tx.done;
    await replayPendingChanges();
}

async function replayPendingChanges(): Promise<void> {
    const db = await openPersonalSyncDB();
    const pending = await db.getAllFromIndex("pending", "by-created");
    for (const change of pending) {
        const payload = change.payload ?? {};
        if (change.entity === "lexicon") {
            const id = String(payload.id ?? "").trim();
            if (!id) continue;
            if (change.operation === "delete") {
                const current = await db.get("lexicon", id);
                if (current) await db.put("lexicon", { ...current, deleted: true, client_updated_at: change.client_updated_at });
            } else {
                await db.put("lexicon", {
                    id, phrase: String(payload.phrase ?? ""), code: String(payload.code ?? ""),
                    shortcut: String(payload.shortcut ?? ""), weight: Number(payload.weight ?? 0),
                    category: String(payload.category ?? ""), notes: String(payload.notes ?? ""),
                    deleted: false, client_updated_at: change.client_updated_at, source_device_id: "local",
                    server_updated_at: 0, version: 1,
                });
            }
        } else if (change.entity === "candidate_usage") {
            const code = String(payload.code ?? "").trim();
            const candidate = String(payload.candidate ?? "").trim();
            const delta = Number(payload.delta ?? 0);
            if (!code || !candidate || delta <= 0) continue;
            const key = candidateUsageKey(code, candidate);
            const current = await db.get("candidateUsage", key);
            await db.put("candidateUsage", { key, code, candidate, count: (current?.count ?? 0) + delta, last_used_at: Number(payload.last_used_at ?? change.client_updated_at) });
        } else if (change.entity === "correction") {
            const original = String(payload.original ?? "").trim();
            const corrected = String(payload.corrected ?? "").trim();
            const delta = Number(payload.delta ?? 0);
            if (!original || !corrected || delta <= 0) continue;
            const key = correctionUsageKey(original, corrected);
            const current = await db.get("corrections", key);
            await db.put("corrections", { key, original, corrected, count: (current?.count ?? 0) + delta, last_used_at: Number(payload.last_used_at ?? change.client_updated_at) });
        } else if (change.entity === "ai_feedback") {
            const feature = String(payload.feature ?? "").trim();
            const contextHash = String(payload.context_hash ?? "").trim();
            if (!feature) continue;
            const key = aiFeedbackKey(feature, contextHash);
            const current = await db.get("aiFeedback", key);
            await db.put("aiFeedback", {
                key, feature, context_hash: contextHash,
                accepted_count: (current?.accepted_count ?? 0) + Number(payload.accepted_delta ?? 0),
                rejected_count: (current?.rejected_count ?? 0) + Number(payload.rejected_delta ?? 0),
                last_used_at: Number(payload.last_used_at ?? change.client_updated_at),
            });
        }
    }
}

async function applyLogChange(change: LogChange, currentDeviceId: string): Promise<void> {
    const db = await openPersonalSyncDB();
    const localMarker = await db.get("localChanges", change.change_id);
    if (change.source_device_id === currentDeviceId && localMarker) {
        await db.delete("localChanges", change.change_id);
        return;
    }
    const payload = change.payload ?? {};
    if (change.entity === "lexicon") {
        const entry = payload as unknown as LexiconEntry;
        if (entry.id) await db.put("lexicon", entry);
        return;
    }
    if (change.entity === "candidate_usage") {
        const code = String(payload.code ?? "").trim();
        const candidate = String(payload.candidate ?? "").trim();
        const delta = Number(payload.delta ?? 0);
        if (!code || !candidate || delta <= 0) return;
        const key = candidateUsageKey(code, candidate);
        const current = await db.get("candidateUsage", key);
        await db.put("candidateUsage", {
            key,
            code,
            candidate,
            count: (current?.count ?? 0) + delta,
            last_used_at: Math.max(current?.last_used_at ?? 0, Number(payload.last_used_at ?? change.created_at)),
        });
        return;
    }
    if (change.entity === "correction") {
        const original = String(payload.original ?? "").trim();
        const corrected = String(payload.corrected ?? "").trim();
        const delta = Number(payload.delta ?? 0);
        if (!original || !corrected || delta <= 0) return;
        const key = correctionUsageKey(original, corrected);
        const current = await db.get("corrections", key);
        await db.put("corrections", {
            key,
            original,
            corrected,
            count: (current?.count ?? 0) + delta,
            last_used_at: Math.max(current?.last_used_at ?? 0, Number(payload.last_used_at ?? change.created_at)),
        });
        return;
    }
    if (change.entity === "ai_feedback") {
        const feature = String(payload.feature ?? "").trim();
        const contextHash = String(payload.context_hash ?? "").trim();
        if (!feature) return;
        const key = aiFeedbackKey(feature, contextHash);
        const current = await db.get("aiFeedback", key);
        await db.put("aiFeedback", {
            key,
            feature,
            context_hash: contextHash,
            accepted_count: (current?.accepted_count ?? 0) + Number(payload.accepted_delta ?? 0),
            rejected_count: (current?.rejected_count ?? 0) + Number(payload.rejected_delta ?? 0),
            last_used_at: Math.max(current?.last_used_at ?? 0, Number(payload.last_used_at ?? change.created_at)),
        });
        return;
    }
    if (change.entity === "setting") {
        const setting = payload as unknown as RemoteSetting;
        if (setting.key) await db.put("remoteSettings", setting);
    }
}

async function pruneLocalMarkers(): Promise<void> {
    const db = await openPersonalSyncDB();
    const cutoff = Date.now() - LOCAL_CHANGE_RETENTION_MS;
    const tx = db.transaction("localChanges", "readwrite");
    let cursor = await tx.store.index("by-created").openCursor(IDBKeyRange.upperBound(cutoff));
    while (cursor) {
        await cursor.delete();
        cursor = await cursor.continue();
    }
    await tx.done;
}

export async function testPersonalSyncConnection(input?: PersonalSyncSettings): Promise<{ ok: true; pending: number; cursor: number }> {
    const settings = input ? normalizePersonalSyncSettings(input) : await loadPersonalSyncSettings();
    if (!settings.apiToken) throw new Error("请先填写 API Token");
    await apiRequest<Record<string, unknown>>(settings, "/api/v1/stats");
    const db = await openPersonalSyncDB();
    return {
        ok: true,
        pending: await db.count("pending"),
        cursor: await getMeta(META_CURSOR, 0),
    };
}

export async function resetPersonalSyncLocalData(): Promise<void> {
    const db = await openPersonalSyncDB();
    const tx = db.transaction([
        "meta", "pending", "localChanges", "lexicon", "candidateUsage", "corrections", "aiFeedback", "remoteSettings",
    ], "readwrite");
    await Promise.all([
        tx.objectStore("meta").clear(),
        tx.objectStore("pending").clear(),
        tx.objectStore("localChanges").clear(),
        tx.objectStore("lexicon").clear(),
        tx.objectStore("candidateUsage").clear(),
        tx.objectStore("corrections").clear(),
        tx.objectStore("aiFeedback").clear(),
        tx.objectStore("remoteSettings").clear(),
    ]);
    await tx.done;
    await savePersonalSyncStatus({ ...kDefaultPersonalSyncStatus });
}

export class PersonalSyncManager {
    private syncPromise: Promise<void> | null = null;
    private idleTimer: ReturnType<typeof setTimeout> | null = null;
    private uploadTimer: ReturnType<typeof setTimeout> | null = null;
    private started = false;

    async start(): Promise<void> {
        if (this.started) return;
        this.started = true;
        const settings = await loadPersonalSyncSettings();
        await this.configureAlarms(settings);
        if (!settings.enabled) {
            await savePersonalSyncStatus({ state: "disabled", error: "" });
            return;
        }
        this.schedulePeriodicUpload(settings.uploadIntervalMs);
        setTimeout(() => void this.syncNow("startup"), 1000);
    }

    async settingsChanged(): Promise<void> {
        const settings = await loadPersonalSyncSettings();
        await this.configureAlarms(settings);
        if (this.uploadTimer) clearTimeout(this.uploadTimer);
        this.uploadTimer = null;
        if (!settings.enabled) {
            await savePersonalSyncStatus({ state: "disabled", error: "" });
            return;
        }
        this.schedulePeriodicUpload(settings.uploadIntervalMs);
        void this.syncNow("settings");
    }

    scheduleIdleFlush(): void {
        if (this.idleTimer) clearTimeout(this.idleTimer);
        void loadPersonalSyncSettings().then((settings) => {
            if (!canUpload(settings) || settings.mode === "manual") return;
            this.idleTimer = setTimeout(() => void this.syncNow("idle"), settings.idleFlushMs);
        });
    }

    async syncNow(reason = "manual"): Promise<void> {
        if (this.syncPromise) return this.syncPromise;
        this.syncPromise = this.runSync(reason).finally(() => {
            this.syncPromise = null;
        });
        return this.syncPromise;
    }

    private async runSync(_reason: string): Promise<void> {
        const settings = await loadPersonalSyncSettings();
        if (!canContactServer(settings)) {
            await savePersonalSyncStatus({
                state: settings.enabled ? "error" : "disabled",
                error: settings.enabled ? "同步设置不完整：请填写服务器地址和 API Token" : "",
            });
            return;
        }
        await savePersonalSyncStatus({ state: "syncing", error: "" });
        try {
            const bootstrapped = await getMeta(META_BOOTSTRAPPED, false);
            if (!bootstrapped) {
                const snapshot = await apiRequest<SnapshotResult>(settings, "/api/v1/sync/snapshot");
                await applySnapshot(snapshot);
            }
            if (canUpload(settings)) {
                await this.pushPending(settings);
            }
            await this.pullChanges(settings);
            await pruneLocalMarkers();
            const cursor = await getMeta(META_CURSOR, 0);
            const pendingCount = await updatePendingStatus();
            await savePersonalSyncStatus({
                state: "idle",
                lastSyncAt: Date.now(),
                pendingCount,
                cursor,
                error: "",
            });
        } catch (error) {
            await updatePendingStatus();
            await savePersonalSyncStatus({
                state: "error",
                error: error instanceof Error ? error.message : String(error),
            });
            throw error;
        }
    }

    private async pushPending(settings: PersonalSyncSettings): Promise<void> {
        const db = await openPersonalSyncDB();
        const all = await db.getAllFromIndex("pending", "by-created");
        if (all.length === 0) return;
        const batch = all.slice(0, settings.batchSize);
        const result = await apiRequest<PushResult>(settings, "/api/v1/sync/push", {
            method: "POST",
            body: JSON.stringify({
                device_id: settings.deviceId,
                device: {
                    name: settings.deviceName,
                    platform: "ChromeOS",
                    app_version: chrome.runtime.getManifest().version,
                },
                changes: batch.map(({ created_at: _createdAt, ...change }) => change),
            }),
        });
        const rejected = new Set((result.rejected ?? []).map((item) => item.change_id));
        const tx = db.transaction("pending", "readwrite");
        for (const item of batch) {
            if (!rejected.has(item.change_id)) await tx.store.delete(item.change_id);
        }
        await tx.done;
        await savePersonalSyncStatus({ lastUploadAt: Date.now() });
        if (result.conflicts > 0) {
            const snapshot = await apiRequest<SnapshotResult>(settings, "/api/v1/sync/snapshot");
            await applySnapshot(snapshot);
        }
        if ((await db.count("pending")) >= settings.batchSize) {
            await this.pushPending(settings);
        }
    }

    private async pullChanges(settings: PersonalSyncSettings): Promise<void> {
        let cursor = await getMeta(META_CURSOR, 0);
        for (let page = 0; page < MAX_PULL_PAGES; page += 1) {
            const result = await apiRequest<PullResult>(settings, `/api/v1/sync/pull?since=${cursor}&limit=500`);
            for (const change of result.changes ?? []) {
                await applyLogChange(change, settings.deviceId);
            }
            cursor = result.cursor ?? cursor;
            await setMeta(META_CURSOR, cursor);
            if (!result.has_more) break;
            if (page === MAX_PULL_PAGES - 1) throw new Error("服务器增量过多，请稍后再次同步");
        }
        await savePersonalSyncStatus({ lastPullAt: Date.now(), cursor });
    }

    private schedulePeriodicUpload(intervalMs: number): void {
        if (this.uploadTimer) clearTimeout(this.uploadTimer);
        this.uploadTimer = setTimeout(() => {
            void this.syncNow("periodic").catch(() => undefined).finally(() => {
                void loadPersonalSyncSettings().then((settings) => {
                    if (settings.enabled) this.schedulePeriodicUpload(settings.uploadIntervalMs);
                });
            });
        }, intervalMs);
    }

    private async configureAlarms(settings: PersonalSyncSettings): Promise<void> {
        if (!chrome.alarms) return;
        await chrome.alarms.clear(PERSONAL_SYNC_ALARM_PULL);
        await chrome.alarms.clear(PERSONAL_SYNC_ALARM_PUSH);
        if (!settings.enabled || settings.mode === "manual") return;
        chrome.alarms.create(PERSONAL_SYNC_ALARM_PULL, {
            periodInMinutes: Math.max(1, settings.pullIntervalMs / 60_000),
        });
        if (settings.mode === "full") {
            chrome.alarms.create(PERSONAL_SYNC_ALARM_PUSH, { periodInMinutes: 1 });
        }
    }
}

export function getPersonalSyncManager(): PersonalSyncManager {
    if (!singletonManager) singletonManager = new PersonalSyncManager();
    return singletonManager;
}

export async function startPersonalSyncBackground(): Promise<void> {
    const manager = getPersonalSyncManager();
    chrome.alarms?.onAlarm.addListener((alarm) => {
        if (alarm.name === PERSONAL_SYNC_ALARM_PULL || alarm.name === PERSONAL_SYNC_ALARM_PUSH) {
            void manager.syncNow(alarm.name).catch(() => undefined);
        }
    });
    chrome.storage.onChanged.addListener((changes, area) => {
        if (area === "local" && changes[PERSONAL_SYNC_STORAGE_KEY]) {
            void manager.settingsChanged();
        }
    });
    await manager.start();
}

export async function syncPersonalDataNow(): Promise<PersonalSyncStatus> {
    await getPersonalSyncManager().syncNow("manual");
    return await loadPersonalSyncStatus();
}

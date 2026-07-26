import { describe, expect, it } from "vitest";

import {
    PERSONAL_SYNC_DEFAULT_SERVER,
    aiFeedbackKey,
    candidateUsageKey,
    correctionUsageKey,
    normalizePersonalSyncSettings,
    normalizeServerUrl,
} from "@/lib/personal-sync";

describe("personal sync settings", () => {
    it("normalizes HTTPS server URLs", () => {
        expect(normalizeServerUrl(" https://shulufa.555044.xyz/ "))
            .toBe("https://shulufa.555044.xyz");
        expect(normalizeServerUrl("https://example.com/sync/"))
            .toBe("https://example.com/sync");
    });

    it("rejects insecure non-local servers", () => {
        expect(normalizeServerUrl("http://example.com"))
            .toBe(PERSONAL_SYNC_DEFAULT_SERVER);
        expect(normalizeServerUrl("http://127.0.0.1:8080/"))
            .toBe("http://127.0.0.1:8080");
    });

    it("normalizes mode, identity and scheduling limits", () => {
        const settings = normalizePersonalSyncSettings({
            enabled: true,
            serverUrl: "https://shulufa.555044.xyz/",
            apiToken: " token ",
            deviceId: " device-1 ",
            deviceName: " Office Chromebook ",
            mode: "download-only",
            uploadIntervalMs: 1,
            pullIntervalMs: 1,
            idleFlushMs: 1,
            batchSize: 9999,
        });

        expect(settings.enabled).toBe(true);
        expect(settings.apiToken).toBe("token");
        expect(settings.deviceId).toBe("device-1");
        expect(settings.deviceName).toBe("Office Chromebook");
        expect(settings.mode).toBe("download-only");
        expect(settings.uploadIntervalMs).toBe(10_000);
        expect(settings.pullIntervalMs).toBe(60_000);
        expect(settings.idleFlushMs).toBe(1_000);
        expect(settings.batchSize).toBe(500);
    });
});

describe("personal sync entity keys", () => {
    it("builds stable compound keys", () => {
        expect(candidateUsageKey(" abcd ", " 测试 "))
            .toBe("abcd\u0000测试");
        expect(correctionUsageKey(" 在确认 ", " 再确认 "))
            .toBe("在确认\u0000再确认");
        expect(aiFeedbackKey(" prediction ", " hash ")).toBe("prediction\u0000hash");
    });
});

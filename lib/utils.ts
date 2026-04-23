import { FastIndexedDbFsController } from './fs'

export function dec2hex(dec: number): string {
    return dec.toString(16).padStart(2, "0")
}

export function generateId(len: number = 40): string {
    const arr = new Uint8Array(len / 2)
    self.crypto.getRandomValues(arr)
    return Array.from(arr, dec2hex).join('')
}

export async function getFs(): Promise<FastIndexedDbFsController> {
    const fs = new FastIndexedDbFsController("rime-files");
    await fs.open();
    return fs;
}

export function formatBytes(bytes: number, decimals = 1): string {
    if (!+bytes) return '0 Bytes'

    const k = 1024
    const dm = decimals < 0 ? 0 : decimals
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']

    const i = Math.floor(Math.log(bytes) / Math.log(k))

    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`
}

export function getParentPath(path: string): string {
    const regex = /^(.*)\/[^/]*$/;
    const match = regex.exec(path);
    return match ? match[1] : "/";
}

export function getFileName(path: string): string | null {
    const regex = /\/([^/]+)$/;
    const match = regex.exec(path);
    return match ? match[1] : null;
}

export interface ImeSettings {
    schema: string;
    pageSize: number;
    algebraList: string[];
    horizontal?: boolean;
    // Ids of schemas visible in the IME tray menu. Undefined = legacy "all downloaded are enabled".
    // The currently active schema is always implicitly enabled regardless of this list.
    enabledSchemas?: string[];
}

export const kDefaultSettings: ImeSettings = { schema: "aurora_pinyin", pageSize: 5, algebraList: [], horizontal: false };

export const $$ = chrome.i18n.getMessage;

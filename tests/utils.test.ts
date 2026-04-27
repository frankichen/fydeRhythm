import { beforeEach, describe, expect, it, vi } from 'vitest';
import { formatBytes, generateId, getFileName, getParentPath } from '../lib/utils';

describe('utility helpers', () => {
    it('formats byte counts with stable units', () => {
        expect(formatBytes(0)).toBe('0 Bytes');
        expect(formatBytes(512)).toBe('512 B');
        expect(formatBytes(1536)).toBe('1.5 KB');
        expect(formatBytes(1024 * 1024, 2)).toBe('1 MB');
    });

    it('gets parent paths for root and nested paths', () => {
        expect(getParentPath('/root/demo/file.txt')).toBe('/root/demo');
        expect(getParentPath('/root')).toBe('');
        expect(getParentPath('relative.txt')).toBe('/');
    });

    it('gets file names from absolute paths', () => {
        expect(getFileName('/root/demo/file.txt')).toBe('file.txt');
        expect(getFileName('/root/demo/')).toBe(null);
        expect(getFileName('relative.txt')).toBe(null);
    });
});

describe('generateId', () => {
    beforeEach(() => {
        vi.stubGlobal('self', {
            crypto: {
                getRandomValues: (array: Uint8Array) => {
                    for (let i = 0; i < array.length; i++) array[i] = i;
                    return array;
                },
            },
        });
    });

    it('produces a lowercase hex string of the correct length', () => {
        // default len=40 → 20 bytes → 40 hex chars
        expect(generateId()).toBe('000102030405060708090a0b0c0d0e0f10111213');
        // custom len=16 → 8 bytes → 16 hex chars
        expect(generateId(16)).toBe('0001020304050607');
    });
});

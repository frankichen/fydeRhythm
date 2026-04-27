import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Entry } from '../lib/fs';

const mocks = vi.hoisted(() => ({
    entries: [] as Entry[],
    getFs: vi.fn(async () => ({
        readAll: async () => mocks.entries,
    })),
}));

vi.mock('@/lib/utils', () => ({
    getFs: mocks.getFs,
}));

const { listEnabledInstalledSchemas } = await import('../entrypoints/background/schemas');

function directory(path: string): Entry {
    return {
        isDirectory: true,
        mtime: 0,
        mode: 0o777,
        blobs: [],
        parent: null,
        fullPath: path,
    };
}

function file(path: string): Entry {
    return {
        ...directory(path),
        isDirectory: false,
    };
}

function stubSchemaCatalog(schemas: Array<{ id: string; name?: string }> = []) {
    const get = vi.fn(async () => ({ schemaList: { schemas } }));
    vi.stubGlobal('chrome', {
        storage: {
            local: { get },
        },
    });
    return get;
}

describe('enabled installed schemas', () => {
    beforeEach(() => {
        mocks.entries = [];
        mocks.getFs.mockClear();
        stubSchemaCatalog();
    });

    it('lists installed schemas in catalog order with catalog labels', async () => {
        mocks.entries = [
            directory('/root/luna'),
            directory('/root/aurora'),
            directory('/root'),
            file('/root/not-a-schema/file.txt'),
        ];
        stubSchemaCatalog([
            { id: 'aurora', name: 'Aurora Pinyin' },
            { id: 'luna', name: 'Luna Pinyin' },
        ]);

        await expect(listEnabledInstalledSchemas('aurora')).resolves.toEqual([
            { id: 'aurora', label: 'Aurora Pinyin' },
            { id: 'luna', label: 'Luna Pinyin' },
        ]);
    });

    it('synthesizes labels for installed schemas missing from the catalog', async () => {
        mocks.entries = [
            directory('/root/catalogued'),
            directory('/root/imported_zip'),
        ];
        stubSchemaCatalog([{ id: 'catalogued', name: 'Catalogued Schema' }]);

        await expect(listEnabledInstalledSchemas('catalogued')).resolves.toEqual([
            { id: 'catalogued', label: 'Catalogued Schema' },
            { id: 'imported_zip', label: 'imported_zip' },
        ]);
    });

    it('filters to enabled schemas while always including the active schema', async () => {
        mocks.entries = [
            directory('/root/active'),
            directory('/root/enabled'),
            directory('/root/disabled'),
        ];
        stubSchemaCatalog([
            { id: 'active', name: 'Active' },
            { id: 'enabled', name: 'Enabled' },
            { id: 'disabled', name: 'Disabled' },
        ]);

        await expect(listEnabledInstalledSchemas('active', ['enabled'])).resolves.toEqual([
            { id: 'active', label: 'Active' },
            { id: 'enabled', label: 'Enabled' },
        ]);
    });

    it('ignores non-schema entries under /root and enabled schemas that are not installed', async () => {
        mocks.entries = [
            directory('/root/installed'),
            directory('/root/installed/build'),
            file('/root/file.schema.yaml'),
            directory('/not-root/other'),
        ];
        stubSchemaCatalog([
            { id: 'installed', name: 'Installed' },
            { id: 'missing', name: 'Missing' },
        ]);

        await expect(listEnabledInstalledSchemas('installed', ['installed', 'missing'])).resolves.toEqual([
            { id: 'installed', label: 'Installed' },
        ]);
    });

    it('returns all installed schemas when enabledSchemas is undefined (legacy mode)', async () => {
        mocks.entries = [
            directory('/root/aurora'),
            directory('/root/luna'),
            directory('/root/rime_ice'),
        ];
        stubSchemaCatalog([
            { id: 'aurora', name: 'Aurora' },
            { id: 'luna', name: 'Luna' },
            { id: 'rime_ice', name: 'Rime Ice' },
        ]);

        // No enabledSchemas argument — all installed schemas must be returned.
        await expect(listEnabledInstalledSchemas('aurora')).resolves.toEqual([
            { id: 'aurora', label: 'Aurora' },
            { id: 'luna', label: 'Luna' },
            { id: 'rime_ice', label: 'Rime Ice' },
        ]);
    });

    it('uses the schema id as label when the catalog entry has no name', async () => {
        mocks.entries = [directory('/root/noname')];
        stubSchemaCatalog([{ id: 'noname' }]);

        await expect(listEnabledInstalledSchemas('noname')).resolves.toEqual([
            { id: 'noname', label: 'noname' },
        ]);
    });

    it('returns uncatalogued installed schemas when schemaList is absent from storage', async () => {
        mocks.entries = [directory('/root/imported')];
        vi.stubGlobal('chrome', {
            storage: {
                local: { get: vi.fn(async () => ({})) },
            },
        });

        await expect(listEnabledInstalledSchemas('imported')).resolves.toEqual([
            { id: 'imported', label: 'imported' },
        ]);
    });
});

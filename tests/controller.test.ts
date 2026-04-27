import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ImeSettings } from '../lib/utils';

const mocks = vi.hoisted(() => ({
    listEnabledInstalledSchemas: vi.fn(),
    fs: {
        readEntryRaw: vi.fn(async () => true),
        createDirectory: vi.fn(async () => { }),
        readWholeFile: vi.fn(async () => new Uint8Array()),
        writeWholeFile: vi.fn(async () => { }),
    },
    getFs: vi.fn(),
    createSession: vi.fn((schema: string) => ({
        addListener: vi.fn(),
        destroy: vi.fn(),
        getContext: vi.fn(async () => null),
        getOption: vi.fn(async () => false),
        getOptionLabel: vi.fn(async () => ''),
        getStatus: vi.fn(async () => ({ schemaId: schema })),
        processKey: vi.fn(async () => true),
        setOption: vi.fn(async () => { }),
    })),
    createEngine: vi.fn(),
    RimeEngine: vi.fn(function () {
        return mocks.createEngine();
    }),
}));

vi.mock('../entrypoints/background/engine', () => ({
    RimeEngine: mocks.RimeEngine,
}));

vi.mock('@/lib/utils', () => ({
    getFs: mocks.getFs,
    kDefaultSettings: {
        schema: 'aurora',
        pageSize: 5,
        algebraList: [],
    },
}));

vi.mock('../entrypoints/background/schemas', () => ({
    listEnabledInstalledSchemas: mocks.listEnabledInstalledSchemas,
}));

const { InputController } = await import('../entrypoints/background/controller');

function stubChrome(settings?: ImeSettings) {
    let storedSettings = settings;
    const get = vi.fn(async () => ({ settings: storedSettings }));
    const set = vi.fn(async (obj: { settings?: ImeSettings }) => {
        if (obj.settings) {
            storedSettings = obj.settings;
        }
    });
    const sendMessage = vi.fn();

    vi.stubGlobal('chrome', {
        i18n: {
            getMessage: (key: string) => key,
        },
        runtime: {
            lastError: null,
            sendMessage,
        },
        storage: {
            sync: { get, set },
        },
        input: {
            ime: {},
        },
    });

    return { get, set, sendMessage };
}

function settings(overrides: Partial<ImeSettings> = {}): ImeSettings {
    return {
        schema: 'aurora',
        pageSize: 5,
        algebraList: [],
        ...overrides,
    };
}

describe('InputController schema switching', () => {
    beforeEach(() => {
        mocks.listEnabledInstalledSchemas.mockReset();
        mocks.fs.readEntryRaw.mockClear();
        mocks.fs.createDirectory.mockClear();
        mocks.fs.readWholeFile.mockClear();
        mocks.fs.writeWholeFile.mockClear();
        mocks.getFs.mockReset();
        mocks.getFs.mockResolvedValue(mocks.fs);
        mocks.createSession.mockClear();
        mocks.createEngine.mockReset();
        mocks.createEngine.mockImplementation(() => ({
            createSession: vi.fn(async (schema: string) => mocks.createSession(schema)),
            destroy: vi.fn(),
            initialize: vi.fn(async () => { }),
            rebuildPrism: vi.fn(async () => { }),
        }));
        mocks.RimeEngine.mockClear();
    });

    it('cycles to the next enabled installed schema and reloads RIME', async () => {
        const chromeMock = stubChrome(settings({
            schema: 'aurora',
            enabledSchemas: ['aurora', 'luna'],
        }));
        mocks.listEnabledInstalledSchemas.mockResolvedValue([
            { id: 'aurora', label: 'Aurora' },
            { id: 'luna', label: 'Luna' },
        ]);
        const controller = new InputController();
        const loadRime = vi.spyOn(controller, 'loadRime').mockResolvedValue(true);

        await controller.cycleToNextSchema();

        expect(mocks.listEnabledInstalledSchemas).toHaveBeenCalledWith('aurora', ['aurora', 'luna']);
        expect(chromeMock.set).toHaveBeenCalledWith({
            settings: {
                schema: 'luna',
                pageSize: 5,
                algebraList: [],
                enabledSchemas: ['aurora', 'luna'],
            },
        });
        expect(loadRime).toHaveBeenCalledWith(true);
    });

    it('wraps from the last schema to the first schema', async () => {
        const chromeMock = stubChrome(settings({ schema: 'luna' }));
        mocks.listEnabledInstalledSchemas.mockResolvedValue([
            { id: 'aurora', label: 'Aurora' },
            { id: 'luna', label: 'Luna' },
        ]);
        const controller = new InputController();
        vi.spyOn(controller, 'loadRime').mockResolvedValue(true);

        await controller.cycleToNextSchema();

        expect(chromeMock.set).toHaveBeenCalledWith({
            settings: {
                schema: 'aurora',
                pageSize: 5,
                algebraList: [],
            },
        });
    });

    it('does not switch when fewer than two schemas are available', async () => {
        const chromeMock = stubChrome(settings({ schema: 'aurora' }));
        mocks.listEnabledInstalledSchemas.mockResolvedValue([{ id: 'aurora', label: 'Aurora' }]);
        const controller = new InputController();
        const loadRime = vi.spyOn(controller, 'loadRime').mockResolvedValue(true);

        await controller.cycleToNextSchema();

        expect(chromeMock.set).not.toHaveBeenCalled();
        expect(loadRime).not.toHaveBeenCalled();
    });

    it('swallows Ctrl+Backquote keydown and cycles schemas', async () => {
        stubChrome(settings());
        mocks.listEnabledInstalledSchemas.mockResolvedValue([
            { id: 'aurora', label: 'Aurora' },
            { id: 'luna', label: 'Luna' },
        ]);
        const controller = new InputController();
        const cycleToNextSchema = vi.spyOn(controller, 'cycleToNextSchema').mockResolvedValue();

        const handled = await controller.feedKey({
            type: 'keydown',
            code: 'Backquote',
            ctrlKey: true,
            altKey: false,
            shiftKey: false,
        } as chrome.input.ime.KeyboardEvent);

        expect(handled).toBe(true);
        expect(cycleToNextSchema).toHaveBeenCalledTimes(1);
    });

    it('cycles from index 0 when the active schema is not in the enabled list', async () => {
        // Simulates a stale setting where the saved schema no longer exists.
        // idx = -1 → clamped to 0 → next = index 1.
        const chromeMock = stubChrome(settings({ schema: 'stale' }));
        mocks.listEnabledInstalledSchemas.mockResolvedValue([
            { id: 'aurora', label: 'Aurora' },
            { id: 'luna', label: 'Luna' },
        ]);
        const controller = new InputController();
        vi.spyOn(controller, 'loadRime').mockResolvedValue(true);

        await controller.cycleToNextSchema();

        expect(chromeMock.set).toHaveBeenCalledWith({
            settings: expect.objectContaining({ schema: 'luna' }),
        });
    });

    it('swallows Ctrl+Backquote keyup without cycling twice', () => {
        stubChrome(settings());
        const controller = new InputController();
        const cycleToNextSchema = vi.spyOn(controller, 'cycleToNextSchema').mockResolvedValue();

        const handled = controller.feedKey({
            type: 'keyup',
            code: 'Backquote',
            ctrlKey: true,
            altKey: false,
            shiftKey: false,
        } as chrome.input.ime.KeyboardEvent);

        expect(handled).toBe(true);
        expect(cycleToNextSchema).not.toHaveBeenCalled();
    });

    it('records the last successfully loaded schema', async () => {
        stubChrome(settings({ schema: 'aurora' }));
        const controller = new InputController();
        vi.spyOn(controller, 'loadRimeConfig').mockResolvedValue('schema: aurora\n');

        await expect(controller.loadRime(false)).resolves.toBe(true);

        expect(controller.lastSuccessfulSchema).toBe('aurora');
    });

    it('falls back to the last successfully loaded schema when a later schema fails', async () => {
        const chromeMock = stubChrome(settings({ schema: 'broken' }));
        const controller = new InputController();
        controller.lastSuccessfulSchema = 'aurora';
        const loadRimeConfig = vi.spyOn(controller, 'loadRimeConfig');
        loadRimeConfig.mockImplementation(async (activeSettings) => {
            if (activeSettings.schema === 'broken') {
                throw new Error('bad schema');
            }
            return 'schema: aurora\n';
        });

        await expect(controller.loadRime(true)).resolves.toBe(true);

        expect(chromeMock.set).toHaveBeenCalledWith({
            settings: {
                schema: 'aurora',
                pageSize: 5,
                algebraList: [],
            },
        });
        expect(loadRimeConfig).toHaveBeenCalledTimes(2);
        expect(loadRimeConfig).toHaveBeenNthCalledWith(1, expect.objectContaining({ schema: 'broken' }));
        expect(loadRimeConfig).toHaveBeenNthCalledWith(2, expect.objectContaining({ schema: 'aurora' }));
        expect(controller.lastSuccessfulSchema).toBe('aurora');
    });

    it('does not retry fallback when there is no previously successful schema', async () => {
        const chromeMock = stubChrome(settings({ schema: 'broken' }));
        const controller = new InputController();
        const loadRimeConfig = vi.spyOn(controller, 'loadRimeConfig').mockRejectedValue(new Error('bad schema'));

        await expect(controller.loadRime(false)).resolves.toBe(false);

        expect(chromeMock.set).not.toHaveBeenCalled();
        expect(loadRimeConfig).toHaveBeenCalledTimes(1);
        expect(chromeMock.sendMessage).toHaveBeenCalledWith({ rimeStatusChanged: true }, {}, expect.any(Function));
    });
});

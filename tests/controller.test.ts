import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ImeSettings } from '../lib/utils';

const mocks = vi.hoisted(() => ({
    listEnabledInstalledSchemas: vi.fn(),
}));

vi.mock('../entrypoints/background/engine', () => ({
    RimeEngine: vi.fn(),
}));

vi.mock('../entrypoints/background/schemas', () => ({
    listEnabledInstalledSchemas: mocks.listEnabledInstalledSchemas,
}));

const { InputController } = await import('../entrypoints/background/controller');

function stubChrome(settings?: ImeSettings) {
    const get = vi.fn(async () => ({ settings }));
    const set = vi.fn(async () => { });
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
});

import { parse } from "yaml";
import { getFs, kDefaultSettings, type ImeSettings } from "@/lib/utils";
import { InputController } from "./controller";
import { serviceWorkerKeepalive } from "./keepalive";
import { onMessage } from "@/lib/messaging";

interface SchemaMetadata {
  id: string;
  name?: string;
}

async function buildSchemaMenuItems(activeSchema: string): Promise<chrome.input.ime.MenuItem[]> {
  const fs = await getFs();
  const entries = await fs.readAll();
  const schemaDirRegex = /^\/root\/([^/]+)$/;
  const installed = new Set<string>(
    entries
      .filter((e: any) => e.isDirectory && schemaDirRegex.test(e.fullPath))
      .map((e: any) => e.fullPath.match(schemaDirRegex)[1])
  );

  const stored = await chrome.storage.local.get(["schemaList"]) as { schemaList?: { schemas?: SchemaMetadata[] } };
  const catalog: SchemaMetadata[] = stored.schemaList?.schemas ?? [];
  const byId = new Map<string, SchemaMetadata>();
  for (const s of catalog) byId.set(s.id, s);

  const items: chrome.input.ime.MenuItem[] = [];
  for (const s of catalog) {
    if (installed.has(s.id)) {
      items.push({
        id: s.id,
        label: s.name || s.id,
        style: "radio",
        visible: true,
        checked: s.id === activeSchema,
        enabled: true,
      });
    }
  }
  // Synthesize entries for directories not in the cached catalog (e.g. ZIP import)
  for (const id of installed) {
    if (!byId.has(id)) {
      items.push({
        id,
        label: id,
        style: "radio",
        visible: true,
        checked: id === activeSchema,
        enabled: true,
      });
    }
  }
  return items;
}

async function refreshImeMenuItems() {
  const engineID = self.controller?.engineId;
  if (!engineID) return;
  let activeSchema = self.controller?.activeSettings?.schema;
  if (!activeSchema) {
    const obj = await chrome.storage.sync.get(["settings"]) as { settings?: ImeSettings };
    activeSchema = obj.settings?.schema ?? "";
  }
  const items = await buildSchemaMenuItems(activeSchema);
  try {
    chrome.input.ime.setMenuItems({ engineID, items });
  } catch (ex) {
    console.error("setMenuItems failed:", ex);
  }
}

export default defineBackground({
  main() {
    async function fallbackDefaultSettings() {
      await chrome.storage.sync.set({ settings: kDefaultSettings });
      await chrome.storage.local.set({
        schemaList: parse(await (await fetch("/builtin/schema-list.yaml")).text())
      });
      if (!self.controller.engine && !self.controller.engineLoading) {
        const schema = kDefaultSettings.schema;
        const schemaBasePath = `/root/${schema}`;
        const fileList = ["aurora_pinyin.prism.bin", "aurora_pinyin.reverse.bin", "aurora_pinyin.table.bin", "aurora_pinyin.schema.yaml"];
        const fs = await getFs();
        for (const f of fileList) {
          const resp = await fetch(`/builtin/${f}`);
          const buf = await resp.arrayBuffer();
          await fs.writeWholeFile(`${schemaBasePath}/build/${f}`, new Uint8Array(buf));
        }
        await self.controller.loadRime(true);
      }
    }

    const postLoad = async () => {
      chrome.input.ime.onFocus.addListener(async (context) => {
        // Todo: in incognito tab, context.shouldDoLearning = false,
        // we should disable rime learning in such context
        console.log("Got focus event, context = ", context);
        self.controller.context = context;
      });

      chrome.input.ime.onBlur.addListener((ctxId) => {
        if (self.controller.context?.contextID == ctxId) {
          self.controller.clearContext();
        }
      });

      chrome.input.ime.onKeyEvent.addListener((engineID: string, keyData: chrome.input.ime.KeyboardEvent, requestId: string) => {
        console.log("Processing key: ", JSON.stringify(keyData));
        const result = self.controller.feedKey(keyData);
        if (result === false || result === true) {
          return result;
        } else {
          result.then((handled) => chrome.input.ime.keyEventHandled(requestId, handled));
          return undefined;
        }
      });

      chrome.input.ime.onCandidateClicked.addListener((engineId, candidateId, button) => {
        console.log(candidateId, button);
        if (button == 'left') {
          self.controller.selectCandidate(candidateId);
          self.controller.lastRightClickItem = -1;
        } else if (button == 'right') {
          self.controller.rightClick(candidateId);
        }
      });

      chrome.input.ime.onMenuItemActivated.addListener(async (engineID, menuId) => {
        const current = self.controller.activeSettings?.schema;
        if (current === menuId) return;
        const obj = await chrome.storage.sync.get(["settings"]) as { settings?: ImeSettings };
        const next: ImeSettings = { ...(obj.settings ?? kDefaultSettings), schema: menuId };
        await chrome.storage.sync.set({ settings: next });
        await self.controller.loadRime(true);
      });
    }

    // Initialize controller
    let rimeLoaded = false;
    self.controller = new InputController();
    self.controller.addListener("schemaSwitched", () => { refreshImeMenuItems(); });

    chrome.storage.sync.get(["settings"]).then(async (obj) => {
      // Only load engine if settings exists
      if (obj.settings) {
        const ok = await self.controller.loadRime(false);
        rimeLoaded = ok;
      }
    }).catch((e) => {
      console.error('load settings error', e);
    }).finally(async () => {
      if (!rimeLoaded) {
        await fallbackDefaultSettings();
      }
      await postLoad();
    });

    // IME activation listener
    chrome.input.ime.onActivate.addListener(async (engineId, screen) => {
      self.controller.engineId = engineId;
      serviceWorkerKeepalive();
      refreshImeMenuItems();
    });

    // Rebuild menu when schemas are installed/removed or active setting changes elsewhere
    chrome.storage.onChanged.addListener((changes, area) => {
      if (area === "local" && (changes.schemaList || changes.selfDefinedSchema)) {
        refreshImeMenuItems();
      } else if (area === "sync" && changes.settings) {
        refreshImeMenuItems();
      }
    });

    // Message handlers using @webext-core/messaging
    onMessage('GetEngineStatus', async () => {
      const loaded = self.controller.engine != null;
      const loading = self.controller.engineLoading;
      let schemaList = [];
      let currentSchema = "";

      if (loaded && !loading) {
        currentSchema = await self.controller.session?.getCurrentSchema();
      }

      return { loading, loaded, schemaList, currentSchema };
    });

    onMessage('GetAsciiMode', async () => {
      try {
        const asciiMode = await self.controller.session?.getOption("ascii_mode");
        return { asciiMode };
      } catch (ex) {
        console.error("Error while getting ascii mode", ex);
        return { asciiMode: true };
      }
    });

    onMessage('GetRimeLogs', () => {
      return { logs: self.controller.getLogs() };
    });

    onMessage('ReloadRime', async () => {
      await self.controller.loadRime(true);
    });

    onMessage('SimulateKey', () => {
      return { handled: false };
    });

    // Port listeners for inputview
    chrome.runtime.onConnect.addListener((port) => {
      if (port.name == "inputviewMessages") {
        console.log("InputView Port Connecting");

        port.onMessage.addListener((msg) => {
          console.log("Message from inputview:", msg);
          if (msg.name == "visibility_change") {
            self.controller.handleInputViewVisibilityChanged(msg.visibility);
          } else if (msg.name == "toggle_language_state") {
            self.controller.setAsciiMode(!msg.msg);
          } else if (msg.name == "select_candidate") {
            self.controller.selectCandidate(msg.candidate.ix, false);
          } else if (msg.name == "load_more_candidate") {
            self.controller.fetchMoreCandidates(msg.more_candidate_count);
          }
        });

        const onToggleLanguageState = function(asciiMode: boolean) {
          port.postMessage({ name: 'front_toggle_language_state', msg: !asciiMode });
        }

        const onCandidatesBack = function(candidates: Array<{ candidate: string, ix: number }>) {
          port.postMessage({ name: "candidates_back", msg: { source: "source", candidates }});
        }

        self.controller.addListener("toggleLanguageState", onToggleLanguageState);
        self.controller.addListener("candidatesBack", onCandidatesBack);

        port.onDisconnect.addListener(() => {
          console.log("InputView disconnected");
          self.controller.removeListener("toggleLanguageState", onToggleLanguageState);
          self.controller.removeListener("candidatesBack", onCandidatesBack);
          self.controller.handleInputViewVisibilityChanged(false);
        });
      }
    });
  }
});

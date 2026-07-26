# ChromeOS AI + Personal Sync release patch

This directory turns an extracted official fydeRhythm 3.0.1 extension into the ChromeOS preview build with:

- local-first IndexedDB cache;
- snapshot and incremental server synchronization;
- background push/pull with retry-safe change IDs;
- complete, download-only and manual modes;
- local personal lexicon management;
- a non-disruptive personal candidate marked with `★`, accepted with `Tab`;
- candidate usage learning without uploading complete input-box contents;
- AI correction and acceptance feedback synchronization;
- stable Rime candidate ordering;
- explicit `Alt+Enter` acceptance for AI candidate recommendations.

The synchronization server defaults to:

```text
https://shulufa.555044.xyz
```

The four standalone runtime scripts are stored in `runtime-patches.tar.gz`. This keeps the generated official Rime runtime out of Git while still making the preview patch reproducible. CI extracts the archive and checks every JavaScript file before release.

## Build from an extracted official extension

Pass the directory containing the official `manifest.json`, `background.js`, `options.html`, and `rime_emscripten.wasm`:

```bash
./build-preview.sh /path/to/fydeRhythm-official \
  /tmp/fydeRhythm-ai-personal-sync-3.0.2.0.zip
```

The build script keeps the generated Rime runtime from the official extension and replaces only the bootstrap and additive patch scripts. The API token is never embedded in the package; users enter it in the options page, where it is stored in `chrome.storage.local`.

## Privacy behavior

The client does not learn from password, URL, email, or `shouldDoLearning=false` contexts. It synchronizes compact learning events and confirmed correction pairs, not complete chat histories.

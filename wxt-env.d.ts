/// <reference types="wxt/browser" />
// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="./.wxt/wxt.d.ts" />

// LESS module declarations
declare module '*.less' {
  const classes: { readonly [key: string]: string };
  export default classes;
}

// Global controller reference (for background script)
declare global {
  var controller: import('./entrypoints/background/controller').InputController;
}

export {};

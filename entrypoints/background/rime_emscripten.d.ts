interface RimeWasmModuleOptions {
  locateFile?: (path: string, directory: string) => string;
  fsc?: unknown;
  idb?: unknown;
  printErr?: (message: string) => void;
  [key: string]: unknown;
}

declare function CreateRimeWasm(options?: RimeWasmModuleOptions): Promise<any>;

export default CreateRimeWasm;

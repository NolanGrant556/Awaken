/// <reference types="vite/client" />

import type { AwakenBridge } from "../shared/runtime";

declare global {
  interface Window {
    awaken?: AwakenBridge;
  }

  interface ImportMetaEnv {
    readonly VITE_AWAKEN_RUNTIME_URL?: string;
  }
}

export {};

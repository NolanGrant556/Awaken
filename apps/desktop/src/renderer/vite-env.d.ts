/// <reference types="vite/client" />

import type { AwakenBridge } from "../shared/runtime";

declare global {
  interface Window {
    awaken?: AwakenBridge;
  }
}

export {};

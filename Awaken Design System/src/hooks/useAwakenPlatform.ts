import { useContext } from "react";
import { AwakenPlatformContext } from "../components/AwakenPlatformProvider";

export function useAwakenPlatform() {
  const context = useContext(AwakenPlatformContext);
  if (!context) {
    throw new Error("useAwakenPlatform must be used within an AwakenPlatformProvider");
  }
  return context;
}

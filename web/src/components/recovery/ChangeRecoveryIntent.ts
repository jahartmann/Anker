import { createContext } from "react";

// Intent only. No historical comparison, decision or target snapshot authorizes a write.
export interface ChangeRecoveryIntent {
  backupID: string;
  manifestSHA: string;
  baselineID: string;
  targetID: string;
  files: string[];
  removeFiles?: string[];
  settingName: string;
  desiredValue: string;
}
export const ChangeRecoveryContext = createContext<
  ((intent: Omit<ChangeRecoveryIntent, "targetID">) => boolean) | null
>(null);

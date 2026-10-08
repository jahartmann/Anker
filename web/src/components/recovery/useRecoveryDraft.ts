import { useEffect, useRef, useState } from "react";
import { api } from "../../api";
import type { Entry, FileDecision, RecoveryInspection } from "../../api";

// In-memory only: navigation retains intent without persisting private contents.
export function useRecoveryDraft(initialBackup = "", initialFile?: string) {
  const [backup, setBackup] = useState(initialBackup);
  const [target, setTarget] = useState("");
  const [scenario, setScenario] = useState("files");
  const [step, setStep] = useState(1);
  const [files, setFiles] = useState<string[]>(
    initialFile ? [initialFile] : [],
  );
  const [entries, setEntries] = useState<Entry[]>([]);
  const [decisions, setDecisions] = useState<Record<string, FileDecision>>({});
  const [interfaces, setInterfaces] = useState<Record<string, string>>({});
  const [storage, setStorage] = useState<Record<string, string>>({});
  const [hostname, setHostname] = useState("");
  const [address, setAddress] = useState("");
  const [consoleOK, setConsoleOK] = useState(false);
  const [offline, setOffline] = useState(false);
  const [inspectionStorageKey, setInspectionStorageKey] = useState("");
  const [inspection, setInspection] = useState<RecoveryInspection | null>(null);
  const [inspecting, setInspecting] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(!!initialBackup);
  const [loadError, setLoadError] = useState("");
  const [reload, setReload] = useState(0);
  const previousInspection = useRef<RecoveryInspection | null>(null);
  const generation = useRef(0);
  const active = useRef(true);
  const inspectionRequest = useRef(0);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      generation.current++;
    };
  }, []);
  useEffect(() => {
    let live = true;
    setEntries([]);
    setLoadError("");
    if (!backup) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api<Entry[]>("backups/" + backup + "/files")
      .then((items) => {
        if (!live) return;
        setEntries(items);
        setFiles((old) =>
          old.filter((p) =>
            items.some((e) => e.path === p && e.type !== "directory"),
          ),
        );
      })
      .catch((e) => {
        if (live) setLoadError(e.message);
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [backup, reload]);
  function invalidate() {
    generation.current++;
    inspectionRequest.current++;
    setInspecting(false);
    setInspection(null);
    setError("");
    setConsoleOK(false);
    setOffline(false);
  }
  function changeBackup(value: string) {
    previousInspection.current = null;
    invalidate();
    setBackup(value);
    setEntries([]);
    setLoading(!!value);
    setLoadError("");
    setFiles([]);
    setDecisions({});
    setInterfaces({});
    setStorage({});
    setHostname("");
    setAddress("");
  }
  function changeTarget(value: string) {
    previousInspection.current = null;
    invalidate();
    setTarget(value);
    setInterfaces({});
    setStorage({});
    setHostname("");
    setAddress("");
    setDecisions((old) =>
      Object.fromEntries(
        Object.entries(old).filter(([, v]) => v === "backup" || v === "manual"),
      ),
    );
  }
  function changeScenario(value: string) {
    invalidate();
    setScenario(value);
    setDecisions((old) =>
      Object.fromEntries(
        Object.entries(old).filter(([, v]) => v !== "prepared"),
      ),
    );
  }
  function changeFiles(value: string[]) {
    invalidate();
    setFiles(value);
    setDecisions((old) =>
      Object.fromEntries(
        Object.entries(old).filter(([p]) => value.includes(p)),
      ),
    );
  }
  const selectedFiles = new Set(files);
  const preserved = new Set(inspection?.preserved?.map((p) => p.path) || []);
  const selectedPaths = entries
    .filter(
      (e) =>
        e.type !== "directory" &&
        !preserved.has(e.path) &&
        (scenario !== "files" || selectedFiles.has(e.path)),
    )
    .map((e) => e.path);
  const storageKey = JSON.stringify(
    Object.entries(storage)
      .filter(
        ([id, value]) => value && inspection?.storage?.some((s) => s.id === id),
      )
      .sort(([a], [b]) => a.localeCompare(b)),
  );
  const needsInspection = !!inspection && inspectionStorageKey !== storageKey;
  function changeStorage(value: Record<string, string>) {
    generation.current++;
    setStorage(value);
    setConsoleOK(false);
    setOffline(false);
  }
  function requestBody() {
    return {
      backup_id: backup,
      target_id: target,
      scenario,
      files: scenario === "files" ? files : [],
      snapshot_id: needsInspection ? undefined : inspection?.snapshot_id,
      decisions: Object.fromEntries(
        selectedPaths.filter((p) => decisions[p]).map((p) => [p, decisions[p]]),
      ),
      mapping: {
        interfaces: Object.fromEntries(
          (inspection?.ports || [])
            .filter((p) => interfaces[p.name])
            .map((p) => [p.name, interfaces[p.name]]),
        ),
        storage: Object.fromEntries(
          (inspection?.storage || [])
            .filter((s) => storage[s.id])
            .map((s) => [s.id, storage[s.id]]),
        ),
        ...(selectedPaths.some((p) =>
          ["etc/hostname", "etc/hosts", "etc/mailname"].includes(p),
        )
          ? { hostname: hostname.trim() }
          : {}),
        ...(inspection?.ports?.length ? { address: address.trim() } : {}),
      },
      console_confirmed: consoleOK,
      source_offline: offline,
    };
  }
  async function inspect() {
    if (
      inspecting ||
      !backup ||
      !target ||
      loading ||
      loadError ||
      (scenario === "files" && !files.length)
    )
      return;
    const id = ++inspectionRequest.current,
      gen = generation.current;
    const inspectedStorageKey = storageKey;
    setInspecting(true);
    setInspection(null);
    setError("");
    setConsoleOK(false);
    setOffline(false);
    try {
      const result = await api<RecoveryInspection>("plans/inspect", "POST", {
        ...requestBody(),
        snapshot_id: undefined,
        decisions: {},
      });
      if (
        !active.current ||
        generation.current !== gen ||
        inspectionRequest.current !== id
      )
        return;
      const previous = previousInspection.current;
      previousInspection.current = result;
      setInspection(result);
      setInspectionStorageKey(inspectedStorageKey);
      setInterfaces((old) =>
        Object.fromEntries(
          (result.ports || []).map((p) => [
            p.name,
            old[p.name] &&
            result.target_ports.some(
              (t) =>
                t.name === old[p.name] &&
                (!previous ||
                  JSON.stringify(t) ===
                    JSON.stringify(
                      previous.target_ports.find(
                        (oldPort) => oldPort.name === t.name,
                      ),
                    )),
            )
              ? old[p.name]
              : p.suggested || "",
          ]),
        ),
      );
      setStorage((old) =>
        Object.fromEntries(
          (result.storage || []).map((s) => [
            s.id,
            old[s.id] === "manual" ||
            result.target_storage.some(
              (t) =>
                t.id === old[s.id] &&
                (!previous ||
                  JSON.stringify(t) ===
                    JSON.stringify(
                      previous.target_storage.find(
                        (oldStorage) => oldStorage.id === t.id,
                      ),
                    )),
            )
              ? old[s.id]
              : "",
          ]),
        ),
      );
      const excluded = new Set(result.preserved?.map((p) => p.path) || []);
      setDecisions((old) =>
        Object.fromEntries(
          Object.entries(old).filter(([p, decision]) => {
            if (excluded.has(p)) return false;
            if (!previous || decision === "backup" || decision === "manual")
              return true;
            return (
              JSON.stringify(previous.files?.find((f) => f.path === p)) ===
              JSON.stringify(result.files?.find((f) => f.path === p))
            );
          }),
        ),
      );
    } catch (e) {
      if (
        active.current &&
        generation.current === gen &&
        inspectionRequest.current === id
      )
        setError((e as Error).message);
    } finally {
      if (active.current && inspectionRequest.current === id)
        setInspecting(false);
    }
  }
  const portValues = (inspection?.ports || [])
    .map((p) => interfaces[p.name])
    .filter(Boolean);
  const duplicatePorts = new Set(portValues).size !== portValues.length;
  const openFiles = selectedPaths.filter((p) => !decisions[p]).length;
  const openMappings =
    (inspection?.ports || []).filter((p) => !interfaces[p.name]).length +
    (inspection?.storage || []).filter((s) => !storage[s.id]).length;
  return {
    backup,
    target,
    scenario,
    step,
    setStep,
    files,
    entries,
    decisions,
    setDecisions,
    interfaces,
    setInterfaces,
    storage,
    setStorage: changeStorage,
    needsInspection,
    snapshotID: needsInspection ? undefined : inspection?.snapshot_id,
    hostname,
    setHostname,
    address,
    setAddress,
    consoleOK,
    setConsoleOK,
    offline,
    setOffline,
    inspection,
    inspecting,
    error,
    setError,
    loading,
    loadError,
    selectedPaths,
    openFiles,
    openMappings,
    duplicatePorts,
    changeBackup,
    changeTarget,
    changeScenario,
    changeFiles,
    requestBody,
    inspect,
    invalidate,
    reloadEntries: () => {
      invalidate();
      setReload((v) => v + 1);
    },
    generation,
    sourceReady:
      !!backup &&
      !loading &&
      !loadError &&
      (scenario !== "files" || files.length > 0),
  };
}
export type RecoveryDraft = ReturnType<typeof useRecoveryDraft>;

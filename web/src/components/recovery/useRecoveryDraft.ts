import type { ChangeRecoveryIntent } from "./ChangeRecoveryIntent";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api";
import type {
  Entry,
  FileDecision,
  RecoveryInspection,
  RecoveryRecipePreview,
  RecoveryNetworkMapping,
} from "../../api";

// In-memory only: navigation retains intent without persisting private contents.
export function useRecoveryDraft(
  initialBackup = "",
  initialFile?: string,
  initialChange?: ChangeRecoveryIntent,
) {
  const [backup, setBackup] = useState(initialBackup);
  const [target, setTarget] = useState(initialChange?.targetID || "");
  const [scenario, setScenario] = useState("files");
  const [step, setStep] = useState(1);
  const [files, setFiles] = useState<string[]>(
    initialChange ? [...initialChange.files] : initialFile ? [initialFile] : [],
  );
  const [removeFiles, setRemoveFiles] = useState<string[]>(
    initialChange?.removeFiles || [],
  );
  const [removalsConfirmed, setRemovalsConfirmed] = useState(false);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [decisions, setDecisions] = useState<Record<string, FileDecision>>({});
  const [interfaces, setInterfaces] = useState<Record<string, string>>({});
  const [storage, setStorage] = useState<Record<string, string>>({});
  const [hostname, setHostname] = useState("");
  const [address, setAddress] = useState("");
  const [network, setNetwork] = useState<
    Record<string, RecoveryNetworkMapping>
  >({});
  const [consoleOK, setConsoleOK] = useState(false);
  const [offline, setOffline] = useState(false);
  const [inspectionStorageKey, setInspectionStorageKey] = useState("");
  const [inspection, setInspection] = useState<RecoveryInspection | null>(null);
  const [inspecting, setInspecting] = useState(false);
  const [recipePreview, setRecipePreview] =
    useState<RecoveryRecipePreview | null>(null);
  const [recipeBusy, setRecipeBusy] = useState(false);
  const recipeRequest = useRef(0);
  const appliedRecipe = useRef(false);
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
        if (
          initialChange &&
          backup === initialChange.backupID &&
          initialChange.files.some(
            (p) => !items.some((e) => e.path === p && e.type !== "directory"),
          )
        ) {
          setLoadError(
            "Eine übergebene Datei fehlt in der Quelle. Änderung erneut öffnen und prüfen.",
          );
          return;
        }
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
  function clearRecipe() {
    recipeRequest.current++;
    setRecipeBusy(false);
    setRecipePreview(null);
  }
  function clearRecipeChoices() {
    if (!appliedRecipe.current) return;
    appliedRecipe.current = false;
    setInterfaces({});
    setStorage({});
    setNetwork({});
    setHostname("");
    setAddress("");
    setDecisions({});
    setRemovalsConfirmed(false);
    setConsoleOK(false);
    setOffline(false);
  }
  function invalidate() {
    clearRecipe();
    clearRecipeChoices();
    generation.current++;
    inspectionRequest.current++;
    setInspecting(false);
    setInspection(null);
    setError("");
    setRemovalsConfirmed(false);
    setConsoleOK(false);
    setOffline(false);
  }
  function changeBackup(value: string) {
    previousInspection.current = null;
    invalidate();
    setBackup(value);
    setRemoveFiles([]);
    setEntries([]);
    setLoading(!!value);
    setLoadError("");
    setFiles([]);
    setDecisions({});
    setInterfaces({});
    setStorage({});
    setHostname("");
    setAddress("");
    setNetwork({});
  }
  function changeTarget(value: string) {
    previousInspection.current = null;
    invalidate();
    setTarget(value);
    setInterfaces({});
    setStorage({});
    setHostname("");
    setAddress("");
    setNetwork({});
    setDecisions({});
  }
  function changeScenario(value: string) {
    invalidate();
    setScenario(value);
    if (value !== "files") setRemoveFiles([]);
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
  const selectedPaths = [
    ...removeFiles,
    ...entries
      .filter(
        (e) =>
          e.type !== "directory" &&
          !preserved.has(e.path) &&
          (scenario !== "files" || selectedFiles.has(e.path)),
      )
      .map((e) => e.path),
  ];
  const required = (paths?: string[]) =>
    !paths?.length ||
    paths.some((p) => selectedPaths.includes(p) && decisions[p] !== "keep");
  const mappingInspection = inspection && {
    ...inspection,
    ports: (inspection.ports || []).filter((p) => required(p.paths)),
    storage: (inspection.storage || []).filter((s) => required(s.paths)),
    network_stanzas: (inspection.network_stanzas || []).filter((s) =>
      required([s.path]),
    ),
  };
  const storageKey = JSON.stringify(
    Object.entries(storage)
      .filter(
        ([id, value]) =>
          value && mappingInspection?.storage?.some((s) => s.id === id),
      )
      .sort(([a], [b]) => a.localeCompare(b)),
  );
  const needsInspection = !!inspection && inspectionStorageKey !== storageKey;
  function changed(mappingChanged = false) {
    clearRecipe();
    generation.current++;
    if (mappingChanged)
      setDecisions((old) =>
        Object.fromEntries(
          Object.entries(old).filter(([, decision]) => decision !== "prepared"),
        ),
      );
    setRemovalsConfirmed(false);
    setConsoleOK(false);
    setOffline(false);
  }
  function changeStorage(value: Record<string, string>) {
    changed(true);
    setStorage(value);
    setRemovalsConfirmed(false);
    setConsoleOK(false);
    setOffline(false);
  }
  function requestBody(preview = false) {
    const context = preview ? inspection : mappingInspection;
    return {
      backup_id: backup,
      target_id: target,
      scenario,
      files: scenario === "files" ? files : [],
      ...(scenario === "files" && removeFiles.length
        ? { remove_files: removeFiles, removals_confirmed: removalsConfirmed }
        : {}),
      snapshot_id: needsInspection ? undefined : inspection?.snapshot_id,
      decisions: preview
        ? {}
        : Object.fromEntries(
            selectedPaths
              .filter((p) => decisions[p])
              .map((p) => [p, decisions[p]]),
          ),
      mapping: {
        interfaces: Object.fromEntries(
          (context?.ports || [])
            .filter((p) => interfaces[p.name])
            .map((p) => [p.name, interfaces[p.name]]),
        ),
        storage: Object.fromEntries(
          (context?.storage || [])
            .filter((s) => storage[s.id])
            .map((s) => [s.id, storage[s.id]]),
        ),
        ...(selectedPaths.some((p) =>
          ["etc/hostname", "etc/hosts", "etc/mailname"].includes(p),
        )
          ? { hostname: hostname.trim() }
          : {}),
        ...(context?.ports?.length ? { address: address.trim() } : {}),
        network: Object.fromEntries(
          (context?.network_stanzas || [])
            .filter((s) => network[s.key])
            .map((s) => [
              s.key,
              {
                ...network[s.key],
                dns_servers: network[s.key].dns_servers?.filter(Boolean),
              },
            ]),
        ),
      },
      console_confirmed: consoleOK,
      source_offline: offline,
    };
  }
  async function previewRecipe(recipeID: string, apply = false) {
    if (!recipeID || !inspection?.snapshot_id || needsInspection) return;
    const id = ++recipeRequest.current,
      gen = generation.current;
    setRecipeBusy(true);
    setRecipePreview(null);
    setError("");
    // Revalidate private evidence on every application; a displayed result is not authority.
    const body = requestBody();
    clearRecipeChoices();
    setRemovalsConfirmed(false);
    setConsoleOK(false);
    setOffline(false);
    try {
      const result = await api<RecoveryRecipePreview>(
        "recovery-recipes/preview",
        "POST",
        {
          recipe_id: recipeID,
          request: {
            ...body,
            decisions: {},
            console_confirmed: false,
            source_offline: false,
          },
        },
      );
      if (
        !active.current ||
        generation.current !== gen ||
        recipeRequest.current !== id
      )
        return;
      if (result.snapshot_id && result.snapshot_id !== body.snapshot_id) return;
      setRecipePreview(result);
      if (apply && result.applicable) {
        generation.current++;
        setInterfaces(result.suggestions.interfaces || {});
        setStorage(result.suggestions.storage || {});
        setNetwork(result.suggestions.network || {});
        setHostname(result.suggestions.hostname || "");
        setAddress(result.suggestions.address || "");
        setDecisions({});
        setRemovalsConfirmed(false);
        setConsoleOK(false);
        setOffline(false);
        appliedRecipe.current = true;
      }
    } catch (e) {
      if (
        active.current &&
        generation.current === gen &&
        recipeRequest.current === id
      )
        setError((e as Error).message);
    } finally {
      if (active.current && recipeRequest.current === id) setRecipeBusy(false);
    }
  }
  async function inspect() {
    if (
      inspecting ||
      !backup ||
      !target ||
      loading ||
      loadError ||
      (scenario === "files" && !files.length && !removeFiles.length)
    )
      return;
    const wasRecipe = appliedRecipe.current;
    const inspectionBody = requestBody();
    if (wasRecipe)
      inspectionBody.mapping = { interfaces: {}, storage: {}, network: {} };
    clearRecipe();
    clearRecipeChoices();
    const id = ++inspectionRequest.current,
      gen = generation.current;
    const inspectedStorageKey = wasRecipe ? "[]" : storageKey;
    setInspecting(true);
    setInspection(null);
    setError("");
    setRemovalsConfirmed(false);
    setConsoleOK(false);
    setOffline(false);
    try {
      const result = await api<RecoveryInspection>("plans/inspect", "POST", {
        ...inspectionBody,
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
      if (
        previous &&
        JSON.stringify(previous.files) !== JSON.stringify(result.files)
      )
        setNetwork({});
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
  const portValues = (mappingInspection?.ports || [])
    .map((p) => interfaces[p.name])
    .filter(Boolean);
  const duplicatePorts = new Set(portValues).size !== portValues.length;
  const openFiles = selectedPaths.filter((p) => !decisions[p]).length;
  const openMappings =
    (mappingInspection?.ports || []).filter((p) => !interfaces[p.name]).length +
    (mappingInspection?.storage || []).filter((s) => !storage[s.id]).length;
  return {
    backup,
    fromChange: !!initialChange && backup === initialChange.backupID,
    target,
    scenario,
    step,
    setStep,
    files,
    removeFiles,
    removalsConfirmed,
    setRemovalsConfirmed,
    changeRemovals: (paths: string[]) => {
      invalidate();
      setRemoveFiles(paths);
      setDecisions({});
    },
    entries,
    decisions,
    setDecisions: (value: Parameters<typeof setDecisions>[0]) => {
      changed();
      setDecisions(value);
    },
    interfaces,
    setInterfaces: (value: Record<string, string>) => {
      changed(true);
      setInterfaces(value);
    },
    storage,
    setStorage: changeStorage,
    needsInspection,
    snapshotID: needsInspection ? undefined : inspection?.snapshot_id,
    hostname,
    setHostname: (value: string) => {
      changed(true);
      setHostname(value);
    },
    address,
    setAddress: (value: string) => {
      changed(true);
      setAddress(value);
    },
    network,
    setNetwork: (value: Record<string, RecoveryNetworkMapping>) => {
      changed(true);
      setNetwork(value);
    },
    mappingInspection,
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
    previewKey: JSON.stringify({
      ...requestBody(true),
      manifest_sha: inspection?.manifest_sha,
      console_confirmed: undefined,
      source_offline: undefined,
      removals_confirmed: undefined,
    }),
    inspect,
    recipePreview,
    recipeBusy,
    previewRecipe,
    clearRecipe,
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
      (scenario !== "files" || files.length > 0 || removeFiles.length > 0),
  };
}
export type RecoveryDraft = ReturnType<typeof useRecoveryDraft>;

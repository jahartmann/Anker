import type { Entry } from "../../api";

export function parentPath(path: string): string {
  const slash = path.lastIndexOf("/");
  return slash < 0 ? "" : path.slice(0, slash);
}

export function indexManifest(entries: Entry[]): Entry[] {
  const indexed = new Map(entries.map((entry) => [entry.path, entry]));
  for (const entry of entries) {
    const parts = entry.path.split("/");
    for (let length = 1; length < parts.length; length++) {
      const path = parts.slice(0, length).join("/");
      if (!indexed.has(path)) {
        indexed.set(path, {
          path,
          type: "directory",
          mode: 0,
          uid: 0,
          gid: 0,
          size: 0,
          secret: false,
        });
      }
    }
  }
  return [...indexed.values()];
}

export function visibleEntries(
  entries: Entry[],
  directory: string,
  query: string,
): Entry[] {
  const search = query.trim().toLowerCase();
  return entries
    .filter((entry) =>
      search
        ? entry.path.toLowerCase().includes(search)
        : parentPath(entry.path) === directory,
    )
    .sort(
      (a, b) =>
        Number(b.type === "directory") - Number(a.type === "directory") ||
        a.path.localeCompare(b.path, undefined, { numeric: true }),
    );
}

/** Maps each directory ("" is the root) to every file below it, at any depth. */
export function filesByDirectory(entries: Entry[]): Map<string, string[]> {
  const result = new Map<string, string[]>([["", []]]);
  for (const entry of entries) {
    if (entry.type === "directory") continue;
    for (
      let directory = parentPath(entry.path);
      ;
      directory = parentPath(directory)
    ) {
      const files = result.get(directory);
      if (files) files.push(entry.path);
      else result.set(directory, [entry.path]);
      if (!directory) break;
    }
  }
  return result;
}

export type GroupSelection = "none" | "some" | "all";

export function groupSelection(
  selected: Set<string>,
  group: string[],
): GroupSelection {
  const count = group.filter((path) => selected.has(path)).length;
  return !count ? "none" : count === group.length ? "all" : "some";
}

/**
 * One click on a folder or "select all" box. A partly selected group becomes
 * fully selected; only a fully selected group is cleared.
 */
export function toggleGroup(selected: string[], group: string[]): string[] {
  const current = new Set(selected);
  if (groupSelection(current, group) === "all") {
    const remove = new Set(group);
    return selected.filter((path) => !remove.has(path));
  }
  return [...selected, ...group.filter((path) => !current.has(path))];
}

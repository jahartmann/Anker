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

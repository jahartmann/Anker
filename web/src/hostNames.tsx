// Presentation only. The catalog name, SSH destination, searches and IDs stay exact.
export function displayHostName(name: string) {
  if (name !== name.toLocaleLowerCase("de")) return name;
  const words = name.split(/[-_]+/).filter(Boolean);
  const prefix = /^(pve|pbs|nas|vm|lxc|db)$/.test(words[0])
    ? words.shift()!.toUpperCase()
    : "";
  const label = words
    .map((w) => w.charAt(0).toLocaleUpperCase("de") + w.slice(1))
    .join(" ");
  return prefix ? (label ? `${prefix} · ${label}` : prefix) : label;
}
export function HostName({ name }: { name: string }) {
  return (
    <span className="host-display-name" title={name} aria-label={name}>
      {displayHostName(name)}
    </span>
  );
}

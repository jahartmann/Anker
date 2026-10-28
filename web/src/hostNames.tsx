// Presentation only. The catalog name, SSH destination, searches and IDs stay exact.
export function displayHostName(name: string) {
  const canonical: Record<string, string> = {
    cp: "CP",
    rs: "RS",
    gs: "GS",
    fz: "FZ",
    gym: "Gym",
    pve: "PVE",
    pbs: "PBS",
    nas: "NAS",
    vm: "VM",
    lxc: "LXC",
    db: "DB",
    ssh: "SSH",
    api: "API",
    ups: "UPS",
    usv: "USV",
    it: "IT",
  };
  const dot = name.indexOf(".");
  const hostname = dot < 0 ? name : name.slice(0, dot);
  const domain = dot < 0 ? "" : name.slice(dot);
  const words = hostname.split(/[-_\s]+/).filter(Boolean);
  const formatted = words.map((word) => {
    const parts = word.match(/^([a-zäöüß]+)(\d*)$/i);
    const key = parts?.[1].toLocaleLowerCase("de") || "";
    const abbreviation = Object.hasOwn(canonical, key)
      ? canonical[key]
      : undefined;
    if (abbreviation) return abbreviation + parts![2];
    // Preserve deliberate casing, including acronyms unknown to Anker.
    if (word !== word.toLocaleLowerCase("de")) return word;
    return word.charAt(0).toLocaleUpperCase("de") + word.slice(1);
  });
  const prefix = /^(pve|pbs|nas|vm|lxc|db)$/i.test(words[0] || "");
  const label =
    prefix && formatted.length > 1
      ? `${formatted[0]} · ${formatted.slice(1).join(" ")}`
      : formatted.join(" ");
  return label + domain;
}
export function HostName({ name }: { name: string }) {
  return (
    <span className="host-display-name" title={name} aria-label={name}>
      {displayHostName(name)}
    </span>
  );
}

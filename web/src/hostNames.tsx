// Presentation only. The catalog name, SSH destination, searches and IDs stay exact.
export function displayHostName(name: string) {
  const canonical: Record<string, string> = {
    cp: "CP",
    zp: "ZP",
    cpu: "CPU",
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
    // Site abbreviations in this inventory also occur without separators.
    // Known ordinary words must not become invented site abbreviations.
    if (!/^(rsync|gymnasium|gymnastik)\d*$/i.test(word)) {
      const site = word.match(
        /^(cp|rs|gs|fz|zp|gym)([a-zäöüß][a-zäöüß0-9]*)$/i,
      );
      if (site) {
        const prefix = canonical[site[1].toLocaleLowerCase("de")];
        const location =
          site[2].charAt(0).toLocaleUpperCase("de") + site[2].slice(1);
        return `${prefix} ${location}`;
      }
    }
    // Preserve deliberate casing, including acronyms unknown to Anker.
    if (word !== word.toLocaleLowerCase("de")) return word;
    return word.charAt(0).toLocaleUpperCase("de") + word.slice(1);
  });
  const prefix = /^(pve|pbs|nas|vm|lxc|db)$/i.test(words[0] || "");
  const label =
    prefix && formatted.length > 1
      ? `${formatted[0]} · ${formatted.slice(1).join(" ")}`
      : formatted.join(" ");
  const withRole =
    !prefix && words.length > 1 && /^backup$/i.test(words[words.length - 1])
      ? `${formatted.slice(0, -1).join(" ")} · Backup`
      : label;
  return withRole + domain;
}
export function HostName({ name }: { name: string }) {
  return (
    <span className="host-display-name" title={name} aria-label={name}>
      {displayHostName(name)}
    </span>
  );
}

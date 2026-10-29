import { useState } from "react";

const sizes = [10, 25, 50, 100, 200];
export function useListPageSize(key: string, initial: number) {
  const storageKey = "anker.list-size." + key;
  const [size, setSize] = useState(() => {
    try {
      const value = Number(localStorage.getItem(storageKey));
      if ([initial, ...sizes].includes(value)) return value;
    } catch {
      /* Browsing remains usable when storage is unavailable. */
    }
    return initial;
  });
  return [
    size,
    (value: number) => {
      if (![initial, ...sizes].includes(value)) return;
      setSize(value);
      try {
        localStorage.setItem(storageKey, String(value));
      } catch {
        /* Optional preference. */
      }
    },
  ] as const;
}
export function PageSizeSelect({
  value,
  onChange,
  label,
  initial = value,
}: {
  value: number;
  onChange: (size: number) => void;
  label: string;
  initial?: number;
}) {
  return (
    <label className="page-size-select">
      <span>Pro Seite</span>
      <select
        aria-label={label}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      >
        {[...new Set([initial, value, ...sizes])]
          .sort((a, b) => a - b)
          .map((size) => (
            <option key={size} value={size}>
              {size} Einträge
            </option>
          ))}
      </select>
    </label>
  );
}

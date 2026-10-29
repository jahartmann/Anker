import { useListPageSize } from "../../listPageSize";
import { useMemo, useState } from "react";
import { Field, Pagination } from "../shared";
export default function RecoveryEvidence({
  title,
  items,
  ordered = false,
}: {
  title: string;
  items: string[];
  ordered?: boolean;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [listSize, setListSize] = useListPageSize("recovery-evidence", 25);
  const matches = useMemo(
    () =>
      items.filter((item) =>
        item.toLowerCase().includes(query.trim().toLowerCase()),
      ),
    [items, query],
  );
  const current = Math.min(
    page,
    Math.max(1, Math.ceil(matches.length / listSize)),
  );
  const rows = matches
    .slice((current - 1) * listSize, current * listSize)
    .map((item, i) => <li key={(current - 1) * 25 + i}>{item}</li>);
  return (
    <details
      className={
        "recovery-disclosure recovery-evidence" +
        (ordered ? " manual-steps" : "")
      }
    >
      <summary>
        {title} · {items.length}
      </summary>
      <section aria-label={title}>
        {items.length > 25 && (
          <Field label={title + " durchsuchen"}>
            <input
              value={query}
              placeholder="Text suchen"
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(1);
              }}
            />
          </Field>
        )}
        {ordered ? (
          <ol start={(current - 1) * 25 + 1}>{rows}</ol>
        ) : (
          <ul>{rows}</ul>
        )}
        {!matches.length && <p className="muted">Keine passenden Einträge.</p>}
        {items.length > 25 && (
          <Pagination
            page={current}
            pageSize={listSize}
            onPageSizeChange={setListSize}
            total={matches.length}
            onPageChange={setPage}
            label={title}
          />
        )}
      </section>
    </details>
  );
}

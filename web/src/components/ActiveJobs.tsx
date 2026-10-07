import { useEffect, useId, useState } from "react";
import { date, labels } from "../api";
import type { Job, Status } from "../api";
import { Pagination, State } from "./shared";

export function jobDuration(job: Job, now = Date.now()) {
  if (!job.started_at) return undefined;
  const start = Date.parse(job.started_at);
  const end = job.finished_at
    ? Date.parse(job.finished_at)
    : job.state === "running"
      ? now
      : NaN;
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start)
    return undefined;
  const seconds = Math.floor((end - start) / 1000);
  if (seconds < 60) return seconds + " Sek.";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return minutes + " Min.";
  const hours = Math.floor(minutes / 60);
  if (hours < 24)
    return (
      hours + " Std." + (minutes % 60 ? " " + (minutes % 60) + " Min." : "")
    );
  return (
    Math.floor(hours / 24) +
    " Tage" +
    (hours % 24 ? " " + (hours % 24) + " Std." : "")
  );
}

export default function ActiveJobs({
  status,
  openJob,
  title = "Aktive Aufträge",
  pageSize = 4,
}: {
  status: Pick<Status, "jobs" | "hosts">;
  openJob?: (id: string) => void;
  title?: string;
  pageSize?: number;
}) {
  const titleID = useId();
  const [page, setPage] = useState(1);
  const [now, setNow] = useState(Date.now);
  const jobs = status.jobs
    .filter((job) => ["queued", "running"].includes(job.state))
    .sort((a, b) => {
      if (a.state !== b.state) return a.state === "running" ? -1 : 1;
      return (
        a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id)
      );
    });
  const running = jobs.filter((job) => job.state === "running").length;
  const size = Math.max(1, pageSize);
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(jobs.length / size)),
  );
  useEffect(() => {
    setPage(currentPage);
  }, [currentPage]);
  useEffect(() => {
    if (!running) return;
    setNow(Date.now());
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [running]);
  if (!jobs.length) return null;
  return (
    <section className="active-jobs" aria-labelledby={titleID}>
      <div className="active-jobs-heading">
        <h3 id={titleID}>{title}</h3>
        <div className="active-jobs-counts">
          {running > 0 && <State value="running" label={running + " laufen"} />}
          {jobs.length > running && (
            <State value="queued" label={jobs.length - running + " warten"} />
          )}
        </div>
      </div>
      <ul className="active-job-list">
        {jobs.slice((currentPage - 1) * size, currentPage * size).map((job) => (
          <li className="active-job" key={job.id}>
            <div className="active-job-summary">
              <strong>
                {status.hosts.find((host) => host.id === job.host_id)?.name ||
                  job.host_id}
              </strong>
              <span className="muted">{labels[job.kind] || job.kind}</span>
            </div>
            <State
              value={job.state}
              label={job.state === "queued" ? "In Warteschlange" : undefined}
            />
            <div className="active-job-meta">
              {job.state === "running" ? (
                <>
                  <span>
                    {jobDuration(job, now)
                      ? "Laufzeit " + jobDuration(job, now)
                      : "Laufzeit nicht erfasst"}
                  </span>
                  <span>Versuch {job.attempts}</span>
                  <span>
                    {job.started_at
                      ? "Gestartet " + date(job.started_at)
                      : "Startzeit nicht erfasst"}
                  </span>
                </>
              ) : (
                <>
                  <span>Wartet auf Ausführung</span>
                  <span>Eingereiht {date(job.created_at)}</span>
                </>
              )}
            </div>
            {openJob && (
              <button
                className="text-button"
                aria-label={`Auftrag ${job.id} ansehen`}
                onClick={() => openJob(job.id)}
              >
                Details
              </button>
            )}
          </li>
        ))}
      </ul>
      {jobs.length > size && (
        <Pagination
          page={currentPage}
          pageSize={size}
          total={jobs.length}
          onPageChange={setPage}
          label={title}
          previousLabel="Vorherige aktive Aufträge"
          nextLabel="Nächste aktive Aufträge"
        />
      )}
    </section>
  );
}

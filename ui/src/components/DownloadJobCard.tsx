import clsx from "clsx";
import { useRef, useEffect, useState } from "react";
import prettyBytes from "pretty-bytes";
import type { Job, JobStatus } from "../utils/apis";
import type { UITranslations } from "../utils/translations";

interface DownloadJobCardProps {
  job: Job;
  onCancel: () => void;
  onClear: () => void;
  t: UITranslations;
}

export function DownloadJobCard({
  job,
  onCancel,
  onClear,
  t,
}: DownloadJobCardProps) {
  const canCancel = job.status === "queued" || job.status === "downloading";
  const canClear =
    job.status === "completed" ||
    job.status === "failed" ||
    job.status === "cancelled";

  // Track download speed with exponential moving average for smoothing
  const prevDownloaded = useRef<number>(0);
  const prevTime = useRef<number>(0);
  const smoothedSpeed = useRef<number>(0);
  const [speed, setSpeed] = useState<number>(0);

  useEffect(() => {
    if (job.status === "downloading") {
      const now = Date.now();
      // Initialize on first update
      if (prevTime.current === 0) {
        prevTime.current = now;
        prevDownloaded.current = job.downloaded;
        return;
      }

      const timeDelta = (now - prevTime.current) / 1000; // seconds
      const bytesDelta = job.downloaded - prevDownloaded.current;

      if (timeDelta > 0 && bytesDelta >= 0) {
        const instantSpeed = bytesDelta / timeDelta;
        // Exponential moving average: higher alpha = more weight on recent data
        const alpha = 0.7;
        smoothedSpeed.current =
          alpha * instantSpeed + (1 - alpha) * smoothedSpeed.current;
        setSpeed(smoothedSpeed.current);
      }

      prevDownloaded.current = job.downloaded;
      prevTime.current = now;
    } else {
      // Reset when not downloading
      prevDownloaded.current = 0;
      prevTime.current = 0;
      smoothedSpeed.current = 0;
      setSpeed(0);
    }
  }, [job.downloaded, job.status]);

  const statusText: Record<JobStatus, string> = {
    queued: t.queued,
    downloading: t.downloading,
    completed: t.completed,
    failed: t.failed,
    cancelled: t.cancelled,
  };

  const statusStyles: Record<JobStatus, string> = {
    queued: "bg-zinc-300 dark:bg-zinc-700 text-zinc-500",
    downloading:
      "bg-blue-100 dark:bg-blue-900/50 text-blue-600 dark:text-blue-400",
    completed:
      "bg-green-100 dark:bg-green-900/50 text-green-600 dark:text-green-500",
    failed: "bg-red-100 dark:bg-red-900/50 text-red-600 dark:text-red-500",
    cancelled: "bg-zinc-300 dark:bg-zinc-700 text-zinc-500 dark:text-zinc-600",
  };

  return (
    <div className="bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-700 rounded-lg p-3 sm:p-4">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-2 mb-2">
        <code className="text-xs text-zinc-400 dark:text-zinc-600 truncate">
          {job.id}
        </code>
        <div className="flex items-center gap-2 flex-wrap">
          <span
            className={clsx(
              "inline-block px-2 py-1 rounded text-[0.65rem] sm:text-[0.7rem] font-medium uppercase",
              statusStyles[job.status]
            )}
          >
            {statusText[job.status]}
          </span>
          {canCancel && (
            <button
              className="px-2 py-1 border border-zinc-300 dark:border-zinc-700 rounded bg-transparent text-zinc-500 dark:text-zinc-600 text-[0.65rem] sm:text-[0.7rem] cursor-pointer hover:border-red-500 hover:text-red-500 transition-colors"
              onClick={onCancel}
            >
              {t.cancel}
            </button>
          )}
          {canClear && (
            <button
              className="px-2 py-1 border border-zinc-300 dark:border-zinc-700 rounded bg-transparent text-zinc-500 text-[0.65rem] sm:text-[0.7rem] cursor-pointer hover:border-red-500 hover:text-red-500 transition-colors"
              onClick={onClear}
            >
              {t.clear_history}
            </button>
          )}
        </div>
      </div>
      <p className="text-sm text-zinc-700 dark:text-zinc-200 break-all mb-2">
        {job.url}
      </p>
      {job.filename && (
        <p className="text-xs text-zinc-400 dark:text-zinc-600 mb-2 overflow-hidden text-ellipsis whitespace-nowrap">
          {job.filename}
        </p>
      )}
      {job.status === "downloading" && (
        <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3 mt-3">
          <div className="flex-1 h-1 bg-zinc-300 dark:bg-zinc-700 rounded overflow-hidden">
            <div
              className={clsx(
                "h-full bg-blue-500 transition-all duration-300",
                job.total <= 0 && "animate-indeterminate"
              )}
              style={{ width: job.total > 0 ? `${job.progress}%` : "100%" }}
            />
          </div>
          <div className="flex flex-wrap justify-between sm:justify-end gap-x-3 gap-y-1 tabular-nums">
            <span className="text-xs text-zinc-400 dark:text-zinc-600 sm:min-w-18 text-left sm:text-right">
              {job.total > 0
                ? `${job.progress.toFixed(1)}%`
                : prettyBytes(job.downloaded)}
            </span>
            <span className="text-xs text-zinc-500 dark:text-zinc-500 sm:min-w-20 text-right">
              {prettyBytes(speed, { bits: false }) + "/s"}
            </span>
            <span className="text-xs text-zinc-500 dark:text-zinc-500 whitespace-nowrap">
              {t.download_concurrency.replace("{count}", String(job.concurrency))}
            </span>
          </div>
        </div>
      )}
      {job.status === "failed" && job.error && (
        <div className="mt-2 p-2 bg-red-100 dark:bg-red-900/30 rounded text-xs text-red-700 dark:text-red-300">
          {job.error}
        </div>
      )}
    </div>
  );
}

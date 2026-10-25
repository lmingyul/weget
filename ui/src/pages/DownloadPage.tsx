import { useRef, useState } from "react";
import clsx from "clsx";
import { useApp } from "../context/AppContext";
import { DownloadJobCard } from "../components/DownloadJobCard";
import { PlaylistDialog } from "../components/PlaylistDialog";
import { postBulkDownload, previewDownload, type DownloadPlaylist } from "../utils/apis";

export function DownloadPage() {
  const {
    isConnected,
    loading,
    jobs,
    outputDir,
    t,
    submitDownload,
    cancelDownload,
    removeJob,
    removeAllJobs,
    updateOutputDir,
    showToast,
    refresh,
  } = useApp();

  const [url, setUrl] = useState("");
  const downloadButtonRef = useRef<HTMLButtonElement>(null);
  const [phase, setPhase] = useState<"checking" | "submitting" | null>(null);
  const submitting = phase !== null;
  const [playlist, setPlaylist] = useState<DownloadPlaylist | null>(null);
  const [playlistError, setPlaylistError] = useState("");
  const [editingDir, setEditingDir] = useState(false);
  const [newOutputDir, setNewOutputDir] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim() || submitting || playlist) return;

    setPhase("checking");
    let checking = true;
    try {
      const preview = await previewDownload(url.trim());
      if (preview.code !== 200) {
        showToast("error", preview.message || t.playlist_check_failed);
        return;
      }
      if (preview.data.playlist && preview.data.playlist.entries.length > 1) {
        setPlaylistError("");
        setPlaylist(preview.data.playlist);
        return;
      }
      checking = false;
      setPhase("submitting");
      const success = await submitDownload(url.trim());
      if (success) {
        setUrl("");
      } else {
        showToast("error", t.playlist_queue_failed);
      }
    } catch {
      showToast("error", checking ? t.playlist_check_failed : t.playlist_queue_failed);
    } finally {
      setPhase(null);
    }
  };

  const handleCurrent = async () => {
    if (!playlist || submitting) return;
    setPhase("submitting");
    setPlaylistError("");
    try {
      if (await submitDownload(playlist.current_url)) {
        setPlaylist(null);
        setUrl("");
      } else {
        setPlaylistError(t.playlist_queue_failed);
      }
    } catch {
      setPlaylistError(t.playlist_queue_failed);
    } finally {
      setPhase(null);
    }
  };

  const handleSelected = async (urls: string[]) => {
    if (!playlist || submitting || urls.length === 0) return;
    setPhase("submitting");
    setPlaylistError("");
    try {
      const res = await postBulkDownload(urls);
      if (res.code !== 200 || res.data.queued === 0) {
        setPlaylistError(res.message || t.playlist_queue_failed);
        return;
      }
      refresh();
      showToast(res.data.failed ? "warning" : "success", res.data.failed
        ? t.playlist_partial.replace("{queued}", String(res.data.queued)).replace("{failed}", String(res.data.failed))
        : `${res.data.queued} ${t.downloads_queued}`);
      setPlaylist(null);
      setUrl("");
    } catch {
      setPlaylistError(t.playlist_queue_failed);
    } finally {
      setPhase(null);
    }
  };

  const handleEditDir = () => {
    setNewOutputDir(outputDir);
    setEditingDir(true);
  };

  const handleSaveDir = async () => {
    if (!newOutputDir.trim()) return;
    const success = await updateOutputDir(newOutputDir.trim());
    if (success) {
      setEditingDir(false);
    }
  };

  const handleCancelEdit = () => {
    setEditingDir(false);
    setNewOutputDir("");
  };

  // Sort by title (filename or URL) for stable ordering
  const sortedJobs = [...jobs].sort((a, b) => {
    const titleA = a.filename || a.url;
    const titleB = b.filename || b.url;
    return titleA.localeCompare(titleB);
  });

  return (
    <div className="max-w-3xl mx-auto flex flex-col gap-4">
      <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
        <span className="text-zinc-700 dark:text-zinc-200 text-sm whitespace-nowrap">
          {t.download_to}
        </span>
        <input
          type="text"
          className={clsx(
            "flex-1 px-3 py-2 border rounded font-mono text-xs sm:text-sm transition-colors focus:outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-600",
            editingDir
              ? "border-blue-500 bg-zinc-100 dark:bg-zinc-950 text-zinc-900 dark:text-white"
              : "border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 text-zinc-700 dark:text-zinc-200 cursor-default"
          )}
          value={editingDir ? newOutputDir : outputDir}
          onChange={(e) => setNewOutputDir(e.target.value)}
          onKeyDown={(e) => {
            if (editingDir && e.key === "Enter") handleSaveDir();
            if (editingDir && e.key === "Escape") handleCancelEdit();
          }}
          readOnly={!editingDir}
          placeholder="..."
        />
        {editingDir ? (
          <div className="flex gap-2 self-end sm:self-auto">
            <button
              onClick={handleSaveDir}
              className="px-3 py-1.5 border border-green-500 text-green-500 rounded text-xs cursor-pointer whitespace-nowrap hover:bg-green-500 hover:text-white transition-colors"
            >
              {t.save}
            </button>
            <button
              onClick={handleCancelEdit}
              className="px-3 py-1.5 border border-zinc-300 dark:border-zinc-700 text-zinc-500 rounded text-xs cursor-pointer whitespace-nowrap hover:border-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors"
            >
              {t.cancel}
            </button>
          </div>
        ) : (
          <button
            onClick={handleEditDir}
            className="px-3 py-1.5 border border-zinc-300 dark:border-zinc-700 text-zinc-500 rounded text-xs cursor-pointer whitespace-nowrap hover:border-blue-500 hover:text-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors self-end sm:self-auto"
            disabled={!isConnected}
          >
            {t.edit}
          </button>
        )}
      </div>

      <form className="flex flex-col sm:flex-row gap-3" onSubmit={handleSubmit}>
        <input
          type="text"
          className="flex-1 px-4 py-3 border border-zinc-300 dark:border-zinc-700 rounded-lg bg-white dark:bg-zinc-900 text-zinc-900 dark:text-white text-base focus:outline-none focus:border-blue-500 placeholder:text-zinc-400 dark:placeholder:text-zinc-600 disabled:opacity-50"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder={t.paste_url}
          disabled={!isConnected || submitting}
        />
        <button
          ref={downloadButtonRef}
          type="submit"
          className="px-6 py-3 border-none rounded-lg bg-blue-500 text-white text-base font-medium cursor-pointer hover:bg-blue-600 disabled:bg-zinc-300 dark:disabled:bg-zinc-700 disabled:cursor-not-allowed transition-colors"
          disabled={!isConnected || !url.trim() || submitting}
        >
          {phase === "checking" ? t.checking_playlist : submitting ? t.adding : t.download}
        </button>
      </form>

      <section className="mt-4">
        <div className="flex items-center gap-3 mb-4">
          <h2 className="text-sm font-medium text-zinc-700 dark:text-zinc-200">
            {t.jobs}
          </h2>
          <span className="text-zinc-700 dark:text-zinc-200 text-sm">
            {jobs.length} {t.total}
          </span>
          <div className="flex gap-2 ml-auto">
            <button
              className="px-2 py-1 border border-zinc-300 dark:border-zinc-700 rounded bg-transparent text-zinc-500 text-[0.7rem] cursor-pointer transition-colors hover:border-red-500 hover:text-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
              onClick={removeAllJobs}
              disabled={
                !isConnected ||
                !jobs.some(
                  (j) =>
                    j.status === "completed" ||
                    j.status === "failed" ||
                    j.status === "cancelled"
                )
              }
              title={t.clear_all}
            >
              {t.clear_all}
            </button>
          </div>
        </div>

        {loading ? (
          <div className="text-center py-12 text-zinc-400 dark:text-zinc-600">
            Loading...
          </div>
        ) : sortedJobs.length === 0 ? (
          <div className="text-center py-12 text-zinc-400 dark:text-zinc-600">
            <p>{t.no_downloads}</p>
            <p className="text-sm mt-2">{t.paste_hint}</p>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {sortedJobs.map((job) => (
              <DownloadJobCard
                key={job.id}
                job={job}
                onCancel={() => cancelDownload(job.id)}
                onClear={() => removeJob(job.id)}
                t={t}
              />
            ))}
          </div>
        )}
      </section>
      {playlist && (
        <PlaylistDialog playlist={playlist} t={t} submitting={submitting} error={playlistError}
          onCurrent={handleCurrent} onSelected={handleSelected} onCancel={() => {
            setPlaylist(null);
            downloadButtonRef.current?.focus();
          }} />
      )}
    </div>
  );
}

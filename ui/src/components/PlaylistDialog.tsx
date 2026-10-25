import { useEffect, useRef, useState } from "react";
import { FaTimes } from "react-icons/fa";
import type { DownloadPlaylist } from "../utils/apis";
import type { UITranslations } from "../utils/translations";

interface PlaylistDialogProps {
  playlist: DownloadPlaylist;
  t: UITranslations;
  submitting: boolean;
  error: string;
  onCurrent: () => void;
  onSelected: (urls: string[]) => void;
  onCancel: () => void;
}

export function PlaylistDialog({ playlist, t, submitting, error, onCurrent, onSelected, onCancel }: PlaylistDialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const selectAllRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState(() => new Set(playlist.entries.map((entry) => entry.url)));
  const allSelected = selected.size === playlist.entries.length;
  const toggleEntry = (url: string, checked: boolean) => {
    setSelected((previous) => {
      const next = new Set(previous);
      if (checked) next.add(url);
      else next.delete(url);
      return next;
    });
  };
  useEffect(() => {
    if (selectAllRef.current) {
      selectAllRef.current.indeterminate = selected.size > 0 && !allSelected;
    }
  }, [selected.size, allSelected]);
  const handleCancel = () => {
    if (submitting) return;
    ref.current?.close();
    onCancel();
  };
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    dialog.showModal();
    return () => dialog.close();
  }, []);

  return (
    <dialog
      ref={ref}
      aria-labelledby="playlist-title"
      aria-describedby="playlist-description"
      className="m-auto w-[calc(100%_-_2rem)] max-w-xl max-h-[calc(100dvh_-_2rem)] overflow-hidden rounded-xl border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 p-0 shadow-xl backdrop:bg-black/50 [color-scheme:light] dark:[color-scheme:dark]"
      onCancel={(event) => {
        event.preventDefault();
        handleCancel();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) handleCancel();
      }}
    >
      <div className="flex max-h-[calc(100dvh_-_2rem)] flex-col">
        <header className="shrink-0 p-5 pb-4 sm:p-6 sm:pb-4">
          <div className="flex items-start justify-between gap-3">
            <h2 id="playlist-title" className="pt-2 text-base font-semibold">{t.playlist_title}</h2>
            <button type="button" aria-label={t.cancel} disabled={submitting} onClick={handleCancel}
              className="flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-800 active:scale-95 focus-visible:outline-2 focus-visible:outline-blue-500 disabled:opacity-50 disabled:cursor-not-allowed">
              <FaTimes aria-hidden="true" />
            </button>
          </div>
          <p className="mt-2 break-words text-pretty text-sm leading-relaxed text-zinc-600 dark:text-zinc-300">{playlist.title}</p>
          <p id="playlist-description" className="mt-2 text-xs leading-relaxed text-zinc-500 dark:text-zinc-400">
            {t.playlist_description.replace("{count}", String(playlist.entries.length))}
          </p>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
            <label className="flex min-h-10 cursor-pointer items-center gap-2 text-sm">
              <input ref={selectAllRef} type="checkbox" checked={allSelected} disabled={submitting}
                onChange={(event) => setSelected(event.target.checked ? new Set(playlist.entries.map((entry) => entry.url)) : new Set())}
                className="h-4 w-4 shrink-0 cursor-pointer rounded accent-blue-500 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-blue-500" />
              {allSelected ? t.playlist_deselect_all : t.playlist_select_all}
            </label>
            <span role="status" className="text-xs tabular-nums text-zinc-500 dark:text-zinc-400">
              {t.playlist_selection.replace("{selected}", String(selected.size)).replace("{total}", String(playlist.entries.length))}
            </span>
          </div>
        </header>
        <ol className="min-h-0 overflow-y-auto overscroll-contain px-5 sm:px-6">
          {playlist.entries.map((entry) => (
            <li key={entry.url} className="border-t border-zinc-100 dark:border-zinc-800">
              <label className="flex min-h-10 cursor-pointer items-start gap-2 py-3">
                <input type="checkbox" checked={selected.has(entry.url)} disabled={submitting}
                  onChange={(event) => toggleEntry(entry.url, event.target.checked)}
                  className="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded accent-blue-500 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-blue-500" />
                <span className="shrink-0 pt-0.5 font-mono text-xs tabular-nums text-zinc-400 dark:text-zinc-500">P{entry.index}</span>
                <span className="min-w-0 flex-1 break-words text-sm leading-relaxed">{entry.title}</span>
                {entry.current && <span className="shrink-0 rounded bg-blue-50 dark:bg-blue-950 px-2 py-1 text-xs text-blue-600 dark:text-blue-300">{t.playlist_current}</span>}
              </label>
            </li>
          ))}
        </ol>
        <footer className="shrink-0 border-t border-zinc-200 dark:border-zinc-800 p-5 sm:p-6">
          {error && <p role="alert" className="mb-3 break-words text-sm text-red-600 dark:text-red-400">{error}</p>}
          <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
            <button type="button" disabled={submitting} onClick={onCurrent} autoFocus
              className="min-h-10 cursor-pointer rounded-lg border border-zinc-300 dark:border-zinc-700 px-4 py-2 text-sm hover:bg-zinc-100 dark:hover:bg-zinc-800 active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-blue-500 disabled:opacity-50 disabled:cursor-not-allowed">
              {t.playlist_single}
            </button>
            <button type="button" disabled={submitting || selected.size === 0}
              onClick={() => onSelected(playlist.entries.filter((entry) => selected.has(entry.url)).map((entry) => entry.url))}
              className="min-h-10 cursor-pointer rounded-lg bg-blue-500 px-4 py-2 text-sm font-medium text-white hover:bg-blue-600 active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-blue-500 focus-visible:outline-offset-2 disabled:opacity-50 disabled:cursor-not-allowed">
              {submitting ? t.adding : t.playlist_download_selected.replace("{count}", String(selected.size))}
            </button>
          </div>
        </footer>
      </div>
    </dialog>
  );
}

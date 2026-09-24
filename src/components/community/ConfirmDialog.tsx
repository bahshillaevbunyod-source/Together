"use client";

import { useEffect } from "react";

import { useLanguage } from "@/lib/language-context";

/**
 * Small confirmation dialog for consequential membership actions. Mirrors the
 * post delete confirmation styling. Cannot be dismissed while `pending`.
 */
export function ConfirmDialog({
  title,
  body,
  confirmLabel,
  pendingLabel,
  pending,
  error,
  onConfirm,
  onCancel,
}: {
  title: string;
  body: string;
  confirmLabel: string;
  pendingLabel: string;
  pending: boolean;
  error: string | null;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const { t } = useLanguage();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !pending) onCancel();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onCancel, pending]);

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40 p-4"
      role="alertdialog"
      aria-modal="true"
      aria-label={title}
      onClick={() => {
        if (!pending) onCancel();
      }}
    >
      <div
        className="w-full max-w-sm rounded-2xl border border-border bg-surface p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="break-words text-base font-semibold text-foreground">{title}</h2>
        <p className="mt-1 text-sm text-muted">{body}</p>
        {error ? (
          <p className="mt-2 text-xs text-red-500" role="alert">
            {error}
          </p>
        ) : null}
        <div className="mt-4 flex items-center justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            className="rounded-full px-4 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-background disabled:opacity-50"
          >
            {t("post.cancel")}
          </button>
          <button
            type="button"
            autoFocus
            onClick={onConfirm}
            disabled={pending}
            className="rounded-full bg-red-600 px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? pendingLabel : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

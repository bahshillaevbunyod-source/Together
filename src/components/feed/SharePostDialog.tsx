"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { X } from "lucide-react";

import {
  getConversations,
  sendMessage,
  type ApiConversation,
} from "@/lib/api";
import { useLanguage } from "@/lib/language-context";

type Status = "loading" | "ready" | "error";

export function SharePostDialog({ postID, onClose }: { postID: string; onClose: () => void }) {
  const { t } = useLanguage();
  const [status, setStatus] = useState<Status>("loading");
  const [conversations, setConversations] = useState<ApiConversation[]>([]);
  const [query, setQuery] = useState("");
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState(false);
  const sendingRef = useRef(false);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getConversations({ limit: 30 }, signal)
      .then((page) => { setConversations(page.items); setStatus("ready"); })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);
  useEffect(() => { const c = new AbortController(); load(c.signal); return () => c.abort(); }, [load]);

  const visible = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return conversations;
    return conversations.filter((c) =>
      c.otherUser.displayName.toLowerCase().includes(normalized) ||
      c.otherUser.username.toLowerCase().includes(normalized),
    );
  }, [conversations, query]);

  const send = async () => {
    if (!selectedID || sendingRef.current) return;
    sendingRef.current = true;
    setSending(true); setSendError(false);
    try {
      const url = new URL(`/post/${postID}`, window.location.origin).toString();
      await sendMessage(selectedID, url);
      onClose();
    } catch {
      setSendError(true);
    } finally {
      sendingRef.current = false;
      setSending(false);
    }
  };

  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-label={t("post.sendInMessage")} onClick={() => !sending && onClose()}>
    <div className="w-full max-w-md rounded-2xl border border-border bg-surface p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
      <div className="flex items-center justify-between gap-3"><h2 className="text-base font-semibold text-foreground">{t("post.sendInMessage")}</h2><button type="button" onClick={onClose} disabled={sending} aria-label={t("post.cancel")} className="rounded-full p-1 text-muted hover:bg-background"><X className="h-5 w-5" /></button></div>
      <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("search.placeholder")} className="mt-4 h-10 w-full rounded-full bg-background px-4 text-sm text-foreground" />
      <div className="mt-3 max-h-64 overflow-y-auto">
        {status === "loading" ? <p className="py-8 text-center text-sm text-muted">{t("messages.loadingConversations")}</p> : null}
        {status === "error" ? <div className="py-8 text-center"><p className="text-sm text-muted">{t("messages.loadError")}</p><button type="button" onClick={() => load()} className="mt-2 text-sm text-primary hover:underline">{t("search.tryAgain")}</button></div> : null}
        {status === "ready" && visible.length === 0 ? <p className="py-8 text-center text-sm text-muted-soft">{t("messages.emptyConversations")}</p> : null}
        {status === "ready" && visible.map((c) => <button key={c.id} type="button" onClick={() => setSelectedID(c.id)} className={`flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left hover:bg-background ${selectedID === c.id ? "bg-primary-soft" : ""}`}><span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">{c.otherUser.displayName}<span className="ml-1 text-muted">@{c.otherUser.username}</span></span></button>)}
      </div>
      {sendError ? <p className="mt-2 text-xs text-red-500" role="alert">{t("messages.sendError")}</p> : null}
      <div className="mt-4 flex justify-end gap-2"><button type="button" onClick={onClose} disabled={sending} className="rounded-full px-4 py-1.5 text-sm text-muted hover:bg-background">{t("post.cancel")}</button><button type="button" onClick={() => void send()} disabled={!selectedID || sending} className="rounded-full bg-primary px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50">{sending ? "…" : t("messages.sendAria")}</button></div>
    </div>
  </div>;
}

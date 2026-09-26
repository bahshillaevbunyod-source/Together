"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";

import { Mic, MoreHorizontal, Paperclip, Pause, Pencil, Play, Send, Square, Trash2, X } from "lucide-react";

import {
  getMessages,
  markConversationRead,
  sendMessage,
	uploadMessageAttachment,
	updateMessage,
	deleteMessage,
  type ApiMessage,
} from "@/lib/api";
import { formatTimeAgo } from "@/lib/format";
import { useRealtime } from "@/lib/realtime-context";
import { useLanguage } from "@/lib/language-context";

type Status = "loading" | "ready" | "error";

interface Props {
  conversationId: string;
  /** The signed-in user's id, to render sender-aware bubbles. */
  currentUserId?: string;
  /** Called after the conversation is marked read, to zero its unread badge. */
  onRead: (conversationId: string) => void;
  /** Called after a message is successfully sent, to update the list. */
  onSent?: (conversationId: string, message: ApiMessage) => void;
  /**
   * Whether the viewer may post (server-provided). When false the composer is
   * replaced by `readOnlyNotice`. Defaults to true (direct messages).
   */
  canPost?: boolean;
  readOnlyNotice?: string;
  /** Show the sender's name above other people's bubbles (groups). */
  showSenders?: boolean;
}
export function ConversationThread({
  conversationId,
  currentUserId,
  onRead,
  onSent,
  canPost = true,
  readOnlyNotice,
  showSenders = false,
}: Props) {
  // Messages held oldest → newest so they read naturally top-to-bottom.
  const [messages, setMessages] = useState<ApiMessage[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [olderCursor, setOlderCursor] = useState("");
  const [loadingOlder, setLoadingOlder] = useState(false);

  // Composer state.
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
	const [attachment, setAttachment] = useState<File | null>(null);
	const [uploading, setUploading] = useState(false);
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [recording, setRecording] = useState(false);
	const [recordingError, setRecordingError] = useState<string | null>(null);
	const [voicePreview, setVoicePreview] = useState<{ file: File; url: string } | null>(null);
	const recorderRef = useRef<MediaRecorder | null>(null);
	const streamRef = useRef<MediaStream | null>(null);
	const chunksRef = useRef<Blob[]>([]);
	const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
	const recordingStartedRef = useRef(0);
	const [elapsed, setElapsed] = useState(0);

	const { subscribeMessageCreated, subscribeMessageUpdated, subscribeMessageDeleted } = useRealtime();
  const { t } = useLanguage();
  const bottomRef = useRef<HTMLDivElement>(null);
  // Tracks message ids currently in the thread, to ignore duplicates.
  const seenIdsRef = useRef<Set<string>>(new Set());
  const loadingOlderRef = useRef(false);

	const stopTracks = useCallback(() => {
		streamRef.current?.getTracks().forEach((track) => track.stop());
		streamRef.current = null;
	}, []);
	const clearRecordingTimer = useCallback(() => {
		if (timerRef.current) clearInterval(timerRef.current);
		timerRef.current = null;
	}, []);
	const discardVoicePreview = useCallback(() => {
		setVoicePreview((current) => { if (current) URL.revokeObjectURL(current.url); return null; });
		setElapsed(0);
	}, []);
	const finishRecording = useCallback((discard: boolean) => {
		clearRecordingTimer();
		const recorder = recorderRef.current;
		if (recorder && recorder.state !== "inactive") recorder.stop();
		recorderRef.current = null;
		stopTracks();
		setRecording(false);
		if (discard) { chunksRef.current = []; discardVoicePreview(); }
	}, [clearRecordingTimer, discardVoicePreview, stopTracks]);
	const startRecording = async () => {
		setRecordingError(null);
		if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") { setRecordingError(t("messages.recordingUnsupported")); return; }
		const mimeType = ["audio/webm", "audio/ogg", "audio/mp4"].find((value) => MediaRecorder.isTypeSupported(value));
		if (!mimeType) { setRecordingError(t("messages.recordingNoFormat")); return; }
		try {
			const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			const recorder = new MediaRecorder(stream, { mimeType });
			chunksRef.current = [];
			streamRef.current = stream;
			recorderRef.current = recorder;
			recorder.ondataavailable = (event) => { if (event.data.size) chunksRef.current.push(event.data); };
			recorder.onerror = () => { setRecordingError(t("messages.recordingFailed")); finishRecording(true); };
			recorder.onstop = () => {
				if (chunksRef.current.length) {
					const blob = new Blob(chunksRef.current, { type: recorder.mimeType || mimeType });
					const ext = (recorder.mimeType || mimeType).includes("ogg") ? "ogg" : (recorder.mimeType || mimeType).includes("mp4") ? "m4a" : "webm";
					const file = new File([blob], `voice.${ext}`, { type: blob.type });
					setAttachment(file);
					setVoicePreview({ file, url: URL.createObjectURL(blob) });
				}
				chunksRef.current = [];
			};
			recorder.start();
			recordingStartedRef.current = Date.now();
			setElapsed(0);
			setRecording(true);
			timerRef.current = setInterval(() => {
				const seconds = Math.floor((Date.now() - recordingStartedRef.current) / 1000);
				setElapsed(seconds);
				if (seconds >= 300) finishRecording(false);
			}, 250);
		} catch (error) {
			setRecordingError(error instanceof DOMException && error.name === "NotAllowedError" ? t("messages.micDenied") : t("messages.micUnavailable"));
			stopTracks();
		}
	};
	useEffect(() => () => { finishRecording(true); discardVoicePreview(); }, [discardVoicePreview, finishRecording]);

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() =>
      bottomRef.current?.scrollIntoView({ behavior: "smooth" }),
    );
  }, []);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getMessages(conversationId, {}, signal)
        .then((page) => {
          // API returns newest first; reverse for natural chat order.
          setMessages(page.items.slice().reverse());
          seenIdsRef.current = new Set(page.items.map((m) => m.id));
          setOlderCursor(page.nextCursor);
          setStatus("ready");
          // Mark read after opening; then clear the unread badge in the list.
          markConversationRead(conversationId)
            .then(() => onRead(conversationId))
            .catch(() => {
              // Non-fatal: the thread still renders.
            });
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    },
    [conversationId, onRead],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadOlder = () => {
    if (loadingOlderRef.current) return;
    const cursor = olderCursor;
    if (!cursor) return;
    loadingOlderRef.current = true;
    setLoadingOlder(true);
    getMessages(conversationId, { cursor })
      .then((page) => {
        // Older page is also newest-first; reverse and prepend.
        setMessages((prev) => [...page.items.slice().reverse(), ...prev]);
        page.items.forEach((m) => seenIdsRef.current.add(m.id));
        setOlderCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => {
        loadingOlderRef.current = false;
        setLoadingOlder(false);
      });
  };

  // Realtime: append incoming messages for THIS conversation only.
  useEffect(() => {
    const unsubscribe = subscribeMessageCreated((event) => {
      if (event.conversationId !== conversationId) return;
      if (seenIdsRef.current.has(event.id)) return; // ignore duplicates
      seenIdsRef.current.add(event.id);

      const incoming: ApiMessage = {
        id: event.id,
        senderId: event.senderId,
        content: event.content,
        createdAt: event.createdAt,
		updatedAt: event.updatedAt ?? event.createdAt,
		deletedAt: null,
		attachment: event.attachment ?? null,
        sender: event.sender ?? null,
        translatedContent: event.translatedContent,
        sourceLanguage: event.sourceLanguage,
        targetLanguage: event.targetLanguage,
      };
      setMessages((prev) => [...prev, incoming]); // preserve oldest→newest
      scrollToBottom();

      // The thread is open, so mark it read; clears the list's unread badge.
      markConversationRead(conversationId)
        .then(() => onRead(conversationId))
        .catch(() => {
          // Non-fatal.
        });
    });
    return unsubscribe;
  }, [subscribeMessageCreated, conversationId, onRead, scrollToBottom]);

	useEffect(() => {
		const stopUpdated = subscribeMessageUpdated((event) => {
			if (event.conversationId !== conversationId) return;
			setMessages((prev) => prev.map((m) => m.id === event.id ? { ...m, ...event } : m));
		});
		const stopDeleted = subscribeMessageDeleted((event) => {
			if (event.conversationId !== conversationId) return;
			setMessages((prev) => prev.map((m) => m.id === event.id ? { ...m, content: "", translatedContent: null, deletedAt: event.deletedAt } : m));
		});
		return () => { stopUpdated(); stopDeleted(); };
	}, [subscribeMessageUpdated, subscribeMessageDeleted, conversationId]);

  const canSend = (text.trim().length > 0 || attachment !== null) && !sending && !uploading;

	const onSend = async (e: React.FormEvent) => {
    e.preventDefault();
		const content = text.trim();
	if ((!content && !attachment) || sending) return; // trim + empty guard + duplicate-submit guard

    setSending(true);
    setSendError(null);
    try {
      let uploaded: { storageKey: string; filename: string } | undefined;
		if (attachment) { setUploading(true); uploaded = await uploadMessageAttachment(attachment, voicePreview ? "voice" : undefined); setUploading(false); }
		const created = await sendMessage(conversationId, content, uploaded);
      // Append only the real server response (no optimistic fake message).
      seenIdsRef.current.add(created.id);
      setMessages((prev) => [...prev, created]);
      setText(""); // clear only on success
		setAttachment(null);
		discardVoicePreview();
      onSent?.(conversationId, created);
      scrollToBottom();
    } catch {
      setSendError(t("messages.sendError")); // keep input on failure
    } finally {
      setUploading(false);
      setSending(false);
    }
  };

  if (status === "loading") {
    return (
      <div className="flex flex-1 items-center justify-center px-6 text-center">
        <p className="text-sm text-muted">{t("messages.loadingMessages")}</p>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 px-6 text-center">
        <p className="text-sm text-muted">{t("messages.messagesLoadError")}</p>
        <button
          type="button"
          onClick={() => load()}
          className="text-sm text-primary hover:underline"
        >
          {t("search.tryAgain")}
        </button>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Messages (history loading/pagination/read logic unchanged) */}
      <div className="flex min-w-0 flex-1 flex-col gap-2 overflow-y-auto px-3 py-4 sm:px-5">
        {messages.length === 0 ? (
          <div className="flex flex-1 items-center justify-center px-1 text-center">
            <p className="text-sm text-muted-soft">{t("messages.threadEmpty")}</p>
          </div>
        ) : (
          <>
            {olderCursor ? (
              <button
                type="button"
                onClick={loadOlder}
                disabled={loadingOlder}
                className="mx-auto mb-2 rounded-full border border-border bg-surface px-4 py-1.5 text-xs text-muted transition-colors hover:text-foreground disabled:opacity-50"
              >
                {loadingOlder ? t("feed.loadingMore") : t("messages.loadOlder")}
              </button>
            ) : null}

            {messages.map((m) => (
              <MessageBubble
                key={m.id}
                message={m}
                mine={currentUserId != null && m.senderId === currentUserId}
                showSender={showSenders}
				onUpdate={(message) => setMessages((prev) => prev.map((m) => m.id === message.id ? message : m))}
				onDelete={(id) => setMessages((prev) => prev.map((m) => m.id === id ? { ...m, content: "", translatedContent: null, deletedAt: new Date().toISOString() } : m))}
              />
            ))}
          </>
        )}
        <div ref={bottomRef} />
      </div>

      {!canPost ? (
        readOnlyNotice ? (
          <div className="border-t border-border px-4 py-3.5 text-center text-xs text-muted">
            {readOnlyNotice}
          </div>
        ) : null
      ) : (
      /* Composer */
      <div className="border-t border-border px-2 py-2.5 sm:px-4 sm:py-3">
        {sendError ? (
          <p className="mb-2 text-xs text-red-500" role="alert">
            {sendError}
          </p>
        ) : null}
        {recordingError ? <p className="mb-2 text-xs text-red-500" role="alert">{recordingError}</p> : null}
        {recording ? <div className="mb-2 flex items-center gap-2 text-xs text-muted"><span className="h-2 w-2 animate-pulse rounded-full bg-red-500" /> <span>{t("messages.recording", { time: formatDuration(elapsed) })}</span><button type="button" onClick={() => finishRecording(false)} className="ml-auto rounded-full px-2 py-1 text-primary hover:bg-background"><Square className="mr-1 inline h-3 w-3" />{t("messages.recordingStop")}</button><button type="button" onClick={() => finishRecording(true)} className="rounded-full px-2 py-1 text-muted hover:bg-background"><X className="mr-1 inline h-3 w-3" />{t("post.cancel")}</button></div> : null}
        {voicePreview ? <VoicePreview file={voicePreview.file} url={voicePreview.url} onDelete={() => { setAttachment(null); discardVoicePreview(); }} /> : null}
        {attachment && !voicePreview ? <div className="mb-2 flex items-center gap-2 text-xs text-muted"><span className="truncate">{attachment.name}</span><button type="button" onClick={() => setAttachment(null)} className="shrink-0 text-primary underline">{t("edit.removePhoto")}</button></div> : null}
        <form onSubmit={onSend} className="flex min-w-0 items-center gap-1 sm:gap-2">
          <input ref={fileInputRef} type="file" accept="image/jpeg,image/png,image/webp,application/pdf,text/plain,text/csv,application/zip" className="hidden" onChange={(e) => { const file = e.target.files?.[0] ?? null; setAttachment(file); e.currentTarget.value = ""; }} />
          <button type="button" aria-label={t("messages.attachFile")} onClick={() => fileInputRef.current?.click()} disabled={sending || uploading} className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"><Paperclip className="h-4 w-4" /></button>
          <button type="button" aria-label={t("messages.recordVoice")} onClick={startRecording} disabled={recording || sending || uploading || voicePreview !== null || typeof MediaRecorder === "undefined"} className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"><Mic className="h-4 w-4" /></button>
          <input
            type="text"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={t("messages.composerPlaceholder")}
            className="h-10 min-w-0 flex-1 rounded-full bg-background px-3 text-base text-foreground placeholder:text-muted-soft sm:px-4 sm:text-sm"
          />
          <button
            type="submit"
            disabled={!canSend}
            aria-label={t("messages.sendAria")}
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
          >
            <Send className="h-4 w-4" />
          </button>
        </form>
      </div>
      )}
    </div>
  );
}

function formatDuration(seconds: number) {
	return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

function VoicePreview({ file, url, onDelete }: { file: File; url: string; onDelete: () => void }) {
	const { t } = useLanguage();
	return <div className="mb-2 flex items-center gap-2 rounded-xl bg-background px-3 py-2 text-xs"><VoicePlayer url={url} /><span className="min-w-0 flex-1 truncate">{file.name}</span><button type="button" aria-label={t("messages.deleteVoicePreview")} onClick={onDelete} className="text-muted hover:text-foreground"><Trash2 className="h-4 w-4" /></button></div>;
}

function VoicePlayer({ url }: { url: string }) {
	const { t } = useLanguage();
	const audioRef = useRef<HTMLAudioElement>(null);
	const [playing, setPlaying] = useState(false);
	const [current, setCurrent] = useState(0);
	const [duration, setDuration] = useState(0);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState(false);
	const toggle = async () => {
		const audio = audioRef.current;
		if (!audio) return;
		setError(false);
		try { if (audio.paused) { if (duration > 0 && current >= duration) audio.currentTime = 0; await audio.play(); } else audio.pause(); } catch { setError(true); }
	};
	return <div className="flex min-w-[170px] items-center gap-2"><audio ref={audioRef} src={url} preload="metadata" className="hidden" onLoadedMetadata={(e) => { setLoading(false); setDuration(Number.isFinite(e.currentTarget.duration) ? e.currentTarget.duration : 0); }} onCanPlay={() => setLoading(false)} onWaiting={() => setLoading(true)} onTimeUpdate={(e) => setCurrent(e.currentTarget.currentTime)} onPlay={() => setPlaying(true)} onPause={() => setPlaying(false)} onEnded={() => { setPlaying(false); setCurrent(0); if (audioRef.current) audioRef.current.currentTime = 0; }} onError={() => { setLoading(false); setError(true); }} /><button type="button" aria-label={playing ? t("messages.pauseVoice") : t("messages.playVoice")} onClick={toggle} disabled={loading || error} className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-white disabled:opacity-50">{playing ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}</button><input aria-label={t("messages.voiceProgress")} type="range" min={0} max={duration || 1} step={0.01} value={Math.min(current, duration || 1)} disabled={loading || error} onChange={(e) => { const next = Number(e.target.value); if (audioRef.current) audioRef.current.currentTime = next; setCurrent(next); }} className="h-1 min-w-0 flex-1 accent-primary" />{error ? <span className="text-[10px] text-red-500">{t("messages.voiceUnavailable")}</span> : loading ? <span className="text-[10px] text-muted">{t("feed.loadingMore")}</span> : <span className="whitespace-nowrap text-[10px] text-muted">{formatDuration(Math.floor(current))}/{formatDuration(Math.floor(duration))}</span>}</div>;
}

// MessageBubble renders one message. When a translation is present it shows the
// translated text by default with a per-message "Show original" toggle. It never
// translates in the browser — it only displays what the backend provided.
function MessageBubble({
  message,
  mine,
  showSender,
	onUpdate,
	onDelete,
}: {
  message: ApiMessage;
  mine: boolean;
  showSender: boolean;
	onUpdate: (message: ApiMessage) => void;
	onDelete: (id: string) => void;
}) {
  const { t, locale } = useLanguage();
  const [showOriginal, setShowOriginal] = useState(false);
	const [editing, setEditing] = useState(false);
	const [draft, setDraft] = useState(message.content);
	const [saving, setSaving] = useState(false);
	const [actionError, setActionError] = useState<string | null>(null);
	// Own-message action menu. Positioned fixed (viewport coordinates) so the
	// thread scroll container can never clip it; closes on outside tap,
	// Escape, scroll or resize.
	const [menuPos, setMenuPos] = useState<{ top: number; left: number } | null>(null);
	const menuOpen = menuPos !== null;
	const menuRef = useRef<HTMLDivElement | null>(null);
	const openMenu = (button: HTMLElement) => {
		const MENU_W = 144, MENU_H = 96, GAP = 4, EDGE = 8;
		const rect = button.getBoundingClientRect();
		const top = rect.top - MENU_H - GAP >= EDGE ? rect.top - MENU_H - GAP : Math.min(rect.bottom + GAP, window.innerHeight - MENU_H - EDGE);
		const left = Math.max(EDGE, Math.min(rect.right - MENU_W, window.innerWidth - MENU_W - EDGE));
		setMenuPos({ top, left });
	};
	useEffect(() => {
		if (!menuOpen) return;
		const close = () => setMenuPos(null);
		const onDown = (e: PointerEvent) => { if (!menuRef.current?.contains(e.target as Node)) close(); };
		const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") close(); };
		document.addEventListener("pointerdown", onDown);
		document.addEventListener("keydown", onKey);
		window.addEventListener("resize", close);
		window.addEventListener("scroll", close, true);
		return () => {
			document.removeEventListener("pointerdown", onDown);
			document.removeEventListener("keydown", onKey);
			window.removeEventListener("resize", close);
			window.removeEventListener("scroll", close, true);
		};
	}, [menuOpen]);
	if (message.deletedAt) return <div className={`flex ${mine ? "justify-end" : "justify-start"}`}><div className="rounded-2xl bg-background px-3.5 py-2 text-sm italic text-muted">{t("messages.deleted")}</div></div>;
	const save = async () => { const content = draft.trim(); if (!content || saving) return; setSaving(true); setActionError(null); try { onUpdate(await updateMessage(message.id, content)); setEditing(false); } catch { setActionError(t("post.editFailed")); } finally { setSaving(false); } };
	const remove = async () => { if (!window.confirm(t("messages.deleteConfirm"))) return; setActionError(null); try { await deleteMessage(message.id); onDelete(message.id); } catch { setActionError(t("messages.deleteFailed")); } };

  const translated = message.translatedContent;
  const hasTranslation =
    translated != null &&
    translated.trim().length > 0 &&
    translated !== message.content;

  const primary =
    hasTranslation && !showOriginal ? (translated as string) : message.content;

  const langHint =
    hasTranslation && message.sourceLanguage && message.targetLanguage
      ? `${message.sourceLanguage.toUpperCase()} → ${message.targetLanguage.toUpperCase()}`
      : null;

  return (
    <div className={`flex ${mine ? "justify-end" : "justify-start"}`}>
      <div
        className={`relative min-w-0 max-w-[85%] rounded-2xl px-3.5 py-2 sm:max-w-[75%] ${
          mine ? "bg-primary text-white" : "bg-background text-foreground"
        }`}
      >
        {message.storyReply ? (
          <p className={`mb-1 flex items-center gap-1 text-[11px] font-medium ${mine ? "text-white/80" : "text-muted"}`}>
            <span aria-hidden>↩</span>
            {mine ? t("messages.storyReplyMine") : t("messages.storyReplyTheirs")}
          </p>
        ) : null}
        {showSender && !mine && message.sender ? (
          <p className="mb-0.5 truncate text-xs font-semibold text-primary">
            {message.sender.displayName}
          </p>
        ) : null}
		{message.attachment ? <div className="mb-1">{message.attachment.type === "image" ? <a href={message.attachment.url} target="_blank" rel="noreferrer"><Image src={message.attachment.url} alt={message.attachment.filename} width={320} height={224} unoptimized className="max-h-56 max-w-full rounded-lg object-contain" /></a> : message.attachment.type === "voice" ? <VoicePlayer url={message.attachment.url} /> : <a href={message.attachment.url} target="_blank" rel="noreferrer" className="flex min-w-0 items-center gap-2 underline"><span>📎</span><span className="min-w-0 truncate">{message.attachment.filename}</span><span className="shrink-0 text-xs opacity-70">{Math.ceil(message.attachment.sizeBytes / 1024)} KB</span></a>}</div> : null}
        {editing ? (
          <div className="flex flex-col gap-2">
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={2}
              autoFocus
              className={`w-full min-w-[12rem] resize-none rounded-lg px-2.5 py-1.5 text-base sm:text-sm ${mine ? "bg-white/20 text-white placeholder:text-white/60" : "bg-surface text-foreground"}`}
            />
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => { setDraft(message.content); setEditing(false); }} className={`h-8 rounded-full px-3 text-xs font-medium ${mine ? "bg-white/15 hover:bg-white/25" : "bg-surface hover:bg-border"}`}>{t("post.cancel")}</button>
              <button type="button" onClick={save} disabled={saving} className={`h-8 rounded-full px-3 text-xs font-semibold disabled:opacity-50 ${mine ? "bg-white text-primary" : "bg-primary text-white"}`}>{t("post.save")}</button>
            </div>
          </div>
        ) : message.content ? <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{primary}</p> : null}
        {actionError ? <p className={`mt-1 break-words text-xs ${mine ? "text-white" : "text-red-500"}`} role="alert">{actionError}</p> : null}

        {/* Meta row wraps instead of spilling; own-message actions live in a menu. */}
        <div
          className={`mt-1 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] leading-4 ${
            mine ? "text-white/75" : "text-muted"
          }`}
        >
          <span className="whitespace-nowrap">{formatTimeAgo(message.createdAt, locale)}</span>
			{message.updatedAt && message.updatedAt !== message.createdAt ? <span className="whitespace-nowrap">· {t("messages.edited")}</span> : null}
          {langHint && !showOriginal ? <span className="whitespace-nowrap">· {langHint}</span> : null}
          {hasTranslation ? (
            <button
              type="button"
              onClick={() => setShowOriginal((v) => !v)}
              className={`whitespace-nowrap font-medium underline underline-offset-2 transition-opacity hover:opacity-80 ${
                mine ? "text-white" : "text-primary"
              }`}
            >
              {showOriginal ? t("post.showTranslation") : t("post.showOriginal")}
            </button>
          ) : null}
          {mine && !editing ? (
            <div ref={menuRef} className="relative ms-auto">
              <button
                type="button"
                onClick={(e) => (menuOpen ? setMenuPos(null) : openMenu(e.currentTarget))}
                aria-label={t("post.more")}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                className="-my-1 -me-1.5 flex h-7 w-7 items-center justify-center rounded-full text-white/80 transition-colors hover:bg-white/15 hover:text-white"
              >
                <MoreHorizontal className="h-4 w-4" aria-hidden />
              </button>
              {menuPos ? (
                <div role="menu" style={{ top: menuPos.top, left: menuPos.left }} className="fixed z-50 w-36 overflow-hidden rounded-xl border border-border bg-surface py-1 text-sm text-foreground shadow-lg">
                  <button type="button" role="menuitem" onClick={() => { setMenuPos(null); setEditing(true); }} className="flex h-11 w-full items-center gap-2 px-3 text-start hover:bg-background">
                    <Pencil className="h-4 w-4 text-muted" aria-hidden />
                    {t("post.edit")}
                  </button>
                  <button type="button" role="menuitem" onClick={() => { setMenuPos(null); void remove(); }} className="flex h-11 w-full items-center gap-2 px-3 text-start text-red-600 hover:bg-red-50">
                    <Trash2 className="h-4 w-4" aria-hidden />
                    {t("post.delete")}
                  </button>
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}


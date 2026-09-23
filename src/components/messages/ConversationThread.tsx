"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";

import { Mic, Paperclip, Pause, Play, Send, Square, Trash2, X } from "lucide-react";

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
}

export function ConversationThread({
  conversationId,
  currentUserId,
  onRead,
  onSent,
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
		if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") { setRecordingError("Voice recording is not supported in this browser."); return; }
		const mimeType = ["audio/webm", "audio/ogg", "audio/mp4"].find((value) => MediaRecorder.isTypeSupported(value));
		if (!mimeType) { setRecordingError("No supported voice recording format is available."); return; }
		try {
			const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			const recorder = new MediaRecorder(stream, { mimeType });
			chunksRef.current = [];
			streamRef.current = stream;
			recorderRef.current = recorder;
			recorder.ondataavailable = (event) => { if (event.data.size) chunksRef.current.push(event.data); };
			recorder.onerror = () => { setRecordingError("Voice recording failed. Please try again."); finishRecording(true); };
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
			setRecordingError(error instanceof DOMException && error.name === "NotAllowedError" ? "Microphone permission was denied." : "Microphone is unavailable.");
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
      <div className="flex flex-1 flex-col gap-2 overflow-y-auto px-5 py-4">
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
				onUpdate={(message) => setMessages((prev) => prev.map((m) => m.id === message.id ? message : m))}
				onDelete={(id) => setMessages((prev) => prev.map((m) => m.id === id ? { ...m, content: "", translatedContent: null, deletedAt: new Date().toISOString() } : m))}
              />
            ))}
          </>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Composer */}
      <div className="border-t border-border px-4 py-3">
        {sendError ? (
          <p className="mb-2 text-xs text-red-500" role="alert">
            {sendError}
          </p>
        ) : null}
        {recordingError ? <p className="mb-2 text-xs text-red-500" role="alert">{recordingError}</p> : null}
        {recording ? <div className="mb-2 flex items-center gap-2 text-xs text-muted"><span className="h-2 w-2 animate-pulse rounded-full bg-red-500" /> <span>Recording {formatDuration(elapsed)}</span><button type="button" onClick={() => finishRecording(false)} className="ml-auto rounded-full px-2 py-1 text-primary hover:bg-background"><Square className="mr-1 inline h-3 w-3" />Stop</button><button type="button" onClick={() => finishRecording(true)} className="rounded-full px-2 py-1 text-muted hover:bg-background"><X className="mr-1 inline h-3 w-3" />Cancel</button></div> : null}
        {voicePreview ? <VoicePreview file={voicePreview.file} url={voicePreview.url} onDelete={() => { setAttachment(null); discardVoicePreview(); }} /> : null}
        {attachment && !voicePreview ? <div className="mb-2 flex items-center gap-2 text-xs text-muted"><span className="truncate">{attachment.name}</span><button type="button" onClick={() => setAttachment(null)} className="text-primary underline">Remove</button></div> : null}
        <form onSubmit={onSend} className="flex items-center gap-2">
          <input ref={fileInputRef} type="file" accept="image/jpeg,image/png,image/webp,application/pdf,text/plain,text/csv,application/zip" className="hidden" onChange={(e) => { const file = e.target.files?.[0] ?? null; setAttachment(file); e.currentTarget.value = ""; }} />
          <button type="button" aria-label="Attach file" onClick={() => fileInputRef.current?.click()} disabled={sending || uploading} className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"><Paperclip className="h-4 w-4" /></button>
          <button type="button" aria-label="Record voice message" onClick={startRecording} disabled={recording || sending || uploading || voicePreview !== null || typeof MediaRecorder === "undefined"} className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"><Mic className="h-4 w-4" /></button>
          <input
            type="text"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={t("messages.composerPlaceholder")}
            className="h-10 flex-1 rounded-full bg-background px-4 text-sm text-foreground placeholder:text-muted-soft"
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
    </div>
  );
}

function formatDuration(seconds: number) {
	return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

function VoicePreview({ file, url, onDelete }: { file: File; url: string; onDelete: () => void }) {
	return <div className="mb-2 flex items-center gap-2 rounded-xl bg-background px-3 py-2 text-xs"><VoicePlayer url={url} /><span className="min-w-0 flex-1 truncate">{file.name}</span><button type="button" aria-label="Delete voice preview" onClick={onDelete} className="text-muted hover:text-foreground"><Trash2 className="h-4 w-4" /></button></div>;
}

function VoicePlayer({ url }: { url: string }) {
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
	return <div className="flex min-w-[170px] items-center gap-2"><audio ref={audioRef} src={url} preload="metadata" className="hidden" onLoadedMetadata={(e) => { setLoading(false); setDuration(Number.isFinite(e.currentTarget.duration) ? e.currentTarget.duration : 0); }} onCanPlay={() => setLoading(false)} onWaiting={() => setLoading(true)} onTimeUpdate={(e) => setCurrent(e.currentTarget.currentTime)} onPlay={() => setPlaying(true)} onPause={() => setPlaying(false)} onEnded={() => { setPlaying(false); setCurrent(0); if (audioRef.current) audioRef.current.currentTime = 0; }} onError={() => { setLoading(false); setError(true); }} /><button type="button" aria-label={playing ? "Pause voice message" : "Play voice message"} onClick={toggle} disabled={loading || error} className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-white disabled:opacity-50">{playing ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}</button><input aria-label="Voice message progress" type="range" min={0} max={duration || 1} step={0.01} value={Math.min(current, duration || 1)} disabled={loading || error} onChange={(e) => { const next = Number(e.target.value); if (audioRef.current) audioRef.current.currentTime = next; setCurrent(next); }} className="h-1 min-w-0 flex-1 accent-primary" />{error ? <span className="text-red-500">Error</span> : loading ? <span className="text-[10px] text-muted">Loading…</span> : <span className="whitespace-nowrap text-[10px] text-muted">{formatDuration(Math.floor(current))}/{formatDuration(Math.floor(duration))}</span>}</div>;
}

// MessageBubble renders one message. When a translation is present it shows the
// translated text by default with a per-message "Show original" toggle. It never
// translates in the browser — it only displays what the backend provided.
function MessageBubble({
  message,
  mine,
	onUpdate,
	onDelete,
}: {
  message: ApiMessage;
  mine: boolean;
	onUpdate: (message: ApiMessage) => void;
	onDelete: (id: string) => void;
}) {
  const { t } = useLanguage();
  const [showOriginal, setShowOriginal] = useState(false);
	const [editing, setEditing] = useState(false);
	const [draft, setDraft] = useState(message.content);
	const [saving, setSaving] = useState(false);
	if (message.deletedAt) return <div className={`flex ${mine ? "justify-end" : "justify-start"}`}><div className="rounded-2xl bg-background px-3.5 py-2 text-sm italic text-muted">Message deleted</div></div>;
	const save = async () => { const content = draft.trim(); if (!content || saving) return; setSaving(true); try { onUpdate(await updateMessage(message.id, content)); setEditing(false); } finally { setSaving(false); } };
	const remove = async () => { if (!window.confirm("Delete this message?")) return; await deleteMessage(message.id); onDelete(message.id); };

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
        className={`max-w-[75%] rounded-2xl px-3.5 py-2 ${
          mine ? "bg-primary text-white" : "bg-background text-foreground"
        }`}
      >
		{message.attachment ? <div className="mb-1">{message.attachment.type === "image" ? <a href={message.attachment.url} target="_blank" rel="noreferrer"><Image src={message.attachment.url} alt={message.attachment.filename} width={320} height={224} unoptimized className="max-h-56 max-w-full rounded-lg object-contain" /></a> : message.attachment.type === "voice" ? <VoicePlayer url={message.attachment.url} /> : <a href={message.attachment.url} target="_blank" rel="noreferrer" className="flex items-center gap-2 underline"><span>📎</span><span className="truncate">{message.attachment.filename}</span><span className="text-xs opacity-70">{Math.ceil(message.attachment.sizeBytes / 1024)} KB</span></a>}</div> : null}
		{editing ? <div className="flex gap-1"><input value={draft} onChange={(e) => setDraft(e.target.value)} className="min-w-0 flex-1 rounded bg-white/20 px-2 py-1 text-sm" /><button type="button" onClick={save} disabled={saving} className="text-xs underline">Save</button><button type="button" onClick={() => { setDraft(message.content); setEditing(false); }} className="text-xs underline">Cancel</button></div> : message.content ? <p className="whitespace-pre-wrap break-words text-sm">{primary}</p> : null}

        <div
          className={`mt-1 flex items-center gap-2 text-[10px] ${
            mine ? "text-white/70" : "text-muted-soft"
          }`}
        >
          <span>{formatTimeAgo(message.createdAt)}</span>
			{message.updatedAt && message.updatedAt !== message.createdAt ? <span>· edited</span> : null}
          {langHint && !showOriginal ? <span>· {langHint}</span> : null}
          {hasTranslation ? (
            <button
              type="button"
              onClick={() => setShowOriginal((v) => !v)}
              className={`underline transition-opacity hover:opacity-80 ${
                mine ? "text-white/80" : "text-primary"
              }`}
            >
              {showOriginal ? t("post.showTranslation") : t("post.showOriginal")}
            </button>
          ) : null}
			{mine && !editing ? <><button type="button" onClick={() => setEditing(true)} className={`underline ${mine ? "text-white/80" : "text-primary"}`}>Edit</button><button type="button" onClick={remove} className={`underline ${mine ? "text-white/80" : "text-primary"}`}>Delete</button></> : null}
        </div>
      </div>
    </div>
  );
}

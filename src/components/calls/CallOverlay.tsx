"use client";

import { useEffect, useState, type ReactNode } from "react";
import Image from "next/image";
import { Mic, MicOff, Phone, PhoneOff, Video, VideoOff, X } from "lucide-react";

import { useCalls, type CallState } from "@/lib/calls/call-context";
import type { CallEndReason } from "@/lib/calls/call-types";
import { useLanguage, type TranslationKey } from "@/lib/language-context";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96"><circle cx="48" cy="48" r="48" fill="#d4d4d8"/></svg>',
  );

const END_REASON_KEY: Record<CallEndReason, TranslationKey> = {
  ended: "calls.ended",
  rejected: "calls.declined",
  busy: "calls.busy",
  cancelled: "calls.cancelled",
  "timeout-outgoing": "calls.noAnswer",
  "timeout-incoming": "calls.missed",
  failed: "calls.failed",
  "mic-denied": "calls.micDenied",
  "camera-denied": "calls.cameraDenied",
  "no-mic": "calls.noMic",
  "no-camera": "calls.noCamera",
  unsupported: "calls.unsupported",
  unavailable: "calls.unavailable",
  "mic-lost": "calls.micLost",
};

function formatDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const mm = h > 0 ? String(m).padStart(2, "0") : String(m);
  return `${h > 0 ? `${h}:` : ""}${mm}:${String(s).padStart(2, "0")}`;
}

/** Live duration from the real connection time; nothing before that. */
function useDuration(startedAt: number | null): string | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (startedAt == null) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [startedAt]);
  return startedAt == null ? null : formatDuration(now - startedAt);
}

/**
 * Binds a MediaStream to a media element and releases it on change/unmount.
 * Uses a callback ref: the element may mount after the stream exists (e.g.
 * when the layout switches to full-screen video).
 */
function useStreamElement<T extends HTMLMediaElement>(stream: MediaStream | null) {
  const [el, setEl] = useState<T | null>(null);
  useEffect(() => {
    if (!el) return;
    el.srcObject = stream;
    if (stream) void el.play().catch(() => {});
    return () => {
      el.srcObject = null;
    };
  }, [el, stream]);
  return setEl;
}

function Avatar({ state, size }: { state: CallState; size: number }) {
  const peer = state.peer;
  return (
    <Image
      src={peer?.avatarUrl ?? FALLBACK_AVATAR}
      alt=""
      width={size}
      height={size}
      unoptimized={Boolean(peer?.avatarUrl)}
      className="shrink-0 rounded-full object-cover"
      style={{ width: size, height: size }}
    />
  );
}

function RoundButton({
  label,
  onClick,
  tone = "neutral",
  children,
  pressed,
}: {
  label: string;
  onClick: () => void;
  tone?: "neutral" | "danger" | "accept";
  pressed?: boolean;
  children: ReactNode;
}) {
  const tones = {
    neutral: pressed
      ? "bg-white text-foreground hover:bg-white/90"
      : "bg-white/15 text-white hover:bg-white/25",
    danger: "bg-red-600 text-white hover:bg-red-700",
    accept: "bg-emerald-600 text-white hover:bg-emerald-700",
  } as const;
  return (
    <div className="flex flex-col items-center gap-1.5">
      <button
        type="button"
        onClick={onClick}
        aria-label={label}
        aria-pressed={pressed}
        className={`flex h-14 w-14 items-center justify-center rounded-full transition-colors ${tones[tone]}`}
      >
        {children}
      </button>
      <span className="line-clamp-2 max-w-[5.5rem] text-center text-[11px] leading-tight text-white/80">{label}</span>
    </div>
  );
}

/**
 * The single call surface, driven entirely by CallProvider state. Ringing and
 * voice calls use a dark card (bottom sheet on mobile); video fills the screen.
 */
export function CallOverlay() {
  const { state, accept, decline, hangUp, toggleMute, toggleCamera, dismiss } = useCalls();
  const { t } = useLanguage();
  const duration = useDuration(state.status === "active" ? state.startedAt : null);
  const remoteVideoRef = useStreamElement<HTMLVideoElement>(state.type === "video" ? state.remoteStream : null);
  const remoteAudioRef = useStreamElement<HTMLAudioElement>(state.type === "voice" ? state.remoteStream : null);
  const localVideoRef = useStreamElement<HTMLVideoElement>(state.type === "video" ? state.localStream : null);

  if (state.status === "idle" || !state.peer) return null;

  const peer = state.peer;
  const typeLabel = t(state.type === "video" ? "calls.videoCall" : "calls.voiceCall");
  const statusLine =
    state.status === "outgoing"
      ? t("calls.calling")
      : state.status === "connecting"
        ? t("calls.connecting")
        : state.status === "active"
          ? state.reconnecting
            ? t("calls.reconnecting")
            : duration
          : null;

  const micButton = (
    <RoundButton label={state.muted ? t("calls.unmute") : t("calls.mute")} onClick={toggleMute} pressed={state.muted}>
      {state.muted ? <MicOff className="h-6 w-6" /> : <Mic className="h-6 w-6" />}
    </RoundButton>
  );
  const endButton = (
    <RoundButton label={t("calls.end")} onClick={hangUp} tone="danger">
      <PhoneOff className="h-6 w-6" />
    </RoundButton>
  );

  // Full-screen video call once media is being negotiated.
  if (state.type === "video" && (state.status === "connecting" || state.status === "active")) {
    return (
      <div
        role="dialog"
        aria-modal="true"
        aria-label={typeLabel}
        className="fixed inset-0 z-[80] flex flex-col bg-foreground text-white"
      >
        <video ref={remoteVideoRef} autoPlay playsInline className="absolute inset-0 h-full w-full object-cover" />
        {!state.remoteStream ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3">
            <Avatar state={state} size={96} />
          </div>
        ) : null}
        <div className="relative flex items-center gap-3 bg-gradient-to-b from-black/60 to-transparent px-4 pb-8 pt-[max(1rem,env(safe-area-inset-top))]">
          <div className="min-w-0">
            <p className="truncate text-base font-semibold">{peer.displayName}</p>
            <p className="text-xs tabular-nums text-white/75" aria-live="polite">
              {statusLine}
            </p>
          </div>
        </div>
        <div className="relative flex-1" />
        {/* Local self-preview (mirrored, fixed). */}
        <div className="absolute right-4 top-[max(4.5rem,calc(env(safe-area-inset-top)+3.5rem))] aspect-[3/4] w-24 overflow-hidden rounded-2xl bg-black/40 shadow-lg ring-1 ring-white/20 sm:w-36">
          <video
            ref={localVideoRef}
            autoPlay
            playsInline
            muted
            className={`h-full w-full -scale-x-100 object-cover ${state.cameraEnabled ? "" : "invisible"}`}
          />
          {!state.cameraEnabled ? (
            <div className="absolute inset-0 flex items-center justify-center">
              <VideoOff className="h-5 w-5 text-white/70" aria-label={t("calls.cameraIsOff")} />
            </div>
          ) : null}
        </div>
        <div className="relative flex items-start justify-center gap-3 bg-gradient-to-t from-black/60 to-transparent px-3 min-[360px]:gap-6 min-[360px]:px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))] pt-10">
          {micButton}
          <RoundButton
            label={state.cameraEnabled ? t("calls.cameraOff") : t("calls.cameraOn")}
            onClick={toggleCamera}
            pressed={!state.cameraEnabled}
          >
            {state.cameraEnabled ? <Video className="h-6 w-6" /> : <VideoOff className="h-6 w-6" />}
          </RoundButton>
          {endButton}
        </div>
      </div>
    );
  }

  let heading: string | null = null;
  let actions: ReactNode = null;
  if (state.status === "incoming") {
    heading = t(state.type === "video" ? "calls.incomingVideo" : "calls.incomingVoice");
    actions = (
      <>
        <RoundButton label={t("calls.decline")} onClick={decline} tone="danger">
          <PhoneOff className="h-6 w-6" />
        </RoundButton>
        <RoundButton label={t("calls.accept")} onClick={accept} tone="accept">
          {state.type === "video" ? <Video className="h-6 w-6" /> : <Phone className="h-6 w-6" />}
        </RoundButton>
      </>
    );
  } else if (state.status === "outgoing") {
    heading = typeLabel;
    actions = endButton;
  } else if (state.status === "connecting" || state.status === "active") {
    heading = typeLabel;
    actions = (
      <>
        {micButton}
        {endButton}
      </>
    );
  } else if (state.status === "ended") {
    heading = state.endReason ? t(END_REASON_KEY[state.endReason]) : t("calls.ended");
  }

  const ringing = state.status === "incoming" || state.status === "outgoing";

  return (
    <div
      role={state.status === "incoming" ? "alertdialog" : "dialog"}
      aria-modal="true"
      aria-label={heading ?? typeLabel}
      className="fixed inset-0 z-[80] flex items-end justify-center bg-black/50 backdrop-blur-sm sm:items-center sm:p-4"
    >
      <audio ref={remoteAudioRef} autoPlay />
      <div className="relative flex w-full flex-col items-center rounded-t-3xl bg-foreground px-6 pb-[max(2rem,env(safe-area-inset-bottom))] pt-10 text-white shadow-2xl sm:max-w-sm sm:rounded-3xl sm:pb-8">
        {state.status === "ended" ? (
          <button
            type="button"
            onClick={dismiss}
            aria-label={t("profile.close")}
            className="absolute right-3 top-3 flex h-9 w-9 items-center justify-center rounded-full text-white/70 hover:bg-white/10"
          >
            <X className="h-5 w-5" />
          </button>
        ) : null}
        <div className={`rounded-full p-1.5 ${ringing ? "animate-pulse ring-2 ring-white/20" : ""}`}>
          <Avatar state={state} size={96} />
        </div>
        <p className="mt-4 max-w-full truncate text-xl font-semibold">{peer.displayName}</p>
        <p className="max-w-full truncate text-sm text-white/60">@{peer.username}</p>
        {heading ? <p className="mt-3 text-center text-sm font-medium text-white/90">{heading}</p> : null}
        {statusLine ? (
          <p className="mt-1 text-sm tabular-nums text-white/70" aria-live="polite">
            {statusLine}
          </p>
        ) : null}
        {actions ? <div className="mt-8 flex items-start justify-center gap-10">{actions}</div> : null}
      </div>
    </div>
  );
}

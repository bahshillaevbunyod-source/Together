"use client";

import { Phone, Video } from "lucide-react";

import { useCalls } from "@/lib/calls/call-context";
import type { CallPeer } from "@/lib/calls/call-types";
import { useLanguage } from "@/lib/language-context";

/**
 * Voice/video call entry points for a direct conversation header. Always
 * clickable for a valid direct conversation: the click itself runs the
 * readiness checks (browser support, fresh GET /calls/config) inside startCall,
 * and any failure is shown as a localized call result — never a silent no-op.
 * Disabled only while a call is already in progress (its overlay is showing).
 */
export function CallButtons({ conversationId, peer }: { conversationId: string; peer: CallPeer }) {
  const { state, startCall } = useCalls();
  const { t } = useLanguage();
  const busy = state.status !== "idle" && state.status !== "ended";

  const base =
    "flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-muted";

  return (
    <div className="flex items-center gap-0.5">
      <button
        type="button"
        onClick={() => startCall({ conversationId, peer }, "voice")}
        disabled={busy}
        aria-label={t("calls.voiceCall")}
        title={t("calls.voiceCall")}
        className={base}
      >
        <Phone className="h-[18px] w-[18px]" />
      </button>
      <button
        type="button"
        onClick={() => startCall({ conversationId, peer }, "video")}
        disabled={busy}
        aria-label={t("calls.videoCall")}
        title={t("calls.videoCall")}
        className={base}
      >
        <Video className="h-5 w-5" />
      </button>
    </div>
  );
}

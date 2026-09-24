"use client";

/**
 * Calls V1 controller: the single owner of call state, the RTCPeerConnection,
 * local/remote media and every call timer. UI components only read state and
 * call the actions below.
 *
 * Invariants:
 * - at most one call (one peer connection) per client;
 * - media is requested only when the user starts or accepts a call, and only
 *   the camera for video calls;
 * - every exit path (end, reject, cancel, busy, timeout, failure, unmount)
 *   runs teardown(): tracks stopped, peer closed, timers cleared;
 * - "ended" always returns to "idle" on its own, so the UI cannot get stuck.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { ApiError } from "@/lib/api";
import { useRealtime, type CallSignalEvent } from "@/lib/realtime-context";
import { createCall, getCallConfig } from "./call-api";
import {
  CONNECT_TIMEOUT_MS,
  ENDED_DISPLAY_MS,
  RING_TIMEOUT_MS,
  type CallDirection,
  type CallEndReason,
  type CallPeer,
  type CallStatus,
  type CallType,
  type OutgoingSignal,
} from "./call-types";

export interface CallState {
  status: CallStatus;
  callId: string | null;
  conversationId: string | null;
  peer: CallPeer | null;
  direction: CallDirection | null;
  type: CallType | null;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  muted: boolean;
  cameraEnabled: boolean;
  /** Epoch ms when media first connected; null before (no fake timer). */
  startedAt: number | null;
  reconnecting: boolean;
  endReason: CallEndReason | null;
}

const IDLE: CallState = {
  status: "idle",
  callId: null,
  conversationId: null,
  peer: null,
  direction: null,
  type: null,
  localStream: null,
  remoteStream: null,
  muted: false,
  cameraEnabled: false,
  startedAt: null,
  reconnecting: false,
  endReason: null,
};

interface CallContextValue {
  state: CallState;
  startCall: (target: { conversationId: string; peer: CallPeer }, type: CallType) => void;
  accept: () => void;
  decline: () => void;
  hangUp: () => void;
  toggleMute: () => void;
  toggleCamera: () => void;
  dismiss: () => void;
}

/** Mutable, non-rendered call resources. Never persisted anywhere. */
interface Session {
  token: number;
  callId: string | null;
  type: CallType;
  pc: RTCPeerConnection | null;
  local: MediaStream | null;
  remote: MediaStream | null;
  offerSdp: string | null;
  remoteReady: boolean;
  pendingCandidates: RTCIceCandidateInit[];
  ringTimer: ReturnType<typeof setTimeout> | null;
  connectTimer: ReturnType<typeof setTimeout> | null;
  trackCleanups: (() => void)[];
}

const CallContext = createContext<CallContextValue | undefined>(undefined);

function isSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.RTCPeerConnection === "function" &&
    typeof navigator !== "undefined" &&
    typeof navigator.mediaDevices?.getUserMedia === "function"
  );
}

function mediaErrorReason(err: unknown, type: CallType): CallEndReason {
  const name = err instanceof DOMException ? err.name : "";
  if (name === "NotAllowedError" || name === "SecurityError") {
    return type === "video" ? "camera-denied" : "mic-denied";
  }
  if (name === "NotFoundError" || name === "OverconstrainedError" || name === "NotReadableError") {
    return type === "video" ? "no-camera" : "no-mic";
  }
  return "failed";
}

function str(v: unknown): string | null {
  return typeof v === "string" && v.length > 0 ? v : null;
}

export function CallProvider({ children }: { children: ReactNode }) {
  const { subscribeCallSignal, sendRealtime } = useRealtime();
  const [state, setState] = useState<CallState>(IDLE);

  const stateRef = useRef<CallState>(IDLE);
  stateRef.current = state;
  const sessionRef = useRef<Session | null>(null);
  const tokenRef = useRef(0);
  const iceServersRef = useRef<RTCIceServer[]>([]);
  const endedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);


  const signal = useCallback(
    (msg: OutgoingSignal) => sendRealtime(msg.type, msg.data),
    [sendRealtime],
  );

  /** Release every resource of the current session. Idempotent. */
  const teardown = useCallback(() => {
    const s = sessionRef.current;
    sessionRef.current = null;
    tokenRef.current += 1; // invalidates in-flight async steps
    if (!s) return;
    if (s.ringTimer) clearTimeout(s.ringTimer);
    if (s.connectTimer) clearTimeout(s.connectTimer);
    s.trackCleanups.forEach((fn) => fn());
    if (s.pc) {
      s.pc.onicecandidate = null;
      s.pc.ontrack = null;
      s.pc.onconnectionstatechange = null;
      try {
        s.pc.close();
      } catch {
        // already closed
      }
    }
    s.local?.getTracks().forEach((t) => t.stop());
    s.remote?.getTracks().forEach((t) => t.stop());
  }, []);

  /** Terminal transition: optional best-effort signal, teardown, summary. */
  const finish = useCallback(
    (reason: CallEndReason, notify?: OutgoingSignal) => {
      if (!sessionRef.current) return;
      if (notify) signal(notify);
      teardown();
      setState((cur) => ({
        ...cur,
        status: "ended",
        endReason: reason,
        localStream: null,
        remoteStream: null,
        reconnecting: false,
      }));
      if (endedTimerRef.current) clearTimeout(endedTimerRef.current);
      endedTimerRef.current = setTimeout(() => {
        endedTimerRef.current = null;
        setState(IDLE);
      }, ENDED_DISPLAY_MS);
    },
    [signal, teardown],
  );

  const endSignal = useCallback((s: Session | null): OutgoingSignal | undefined => {
    return s?.callId ? { type: "call.end", data: { callId: s.callId } } : undefined;
  }, []);

  const clearEndedSummary = () => {
    if (endedTimerRef.current) {
      clearTimeout(endedTimerRef.current);
      endedTimerRef.current = null;
    }
  };

  const acquireMedia = async (type: CallType): Promise<MediaStream> => {
    return navigator.mediaDevices.getUserMedia({
      audio: true,
      video: type === "video" ? { facingMode: "user" } : false,
    });
  };

  /** Watch local tracks for devices that disappear mid-call. */
  const watchLocalTracks = useCallback(
    (s: Session, stream: MediaStream) => {
      for (const track of stream.getTracks()) {
        const onEnded = () => {
          if (sessionRef.current !== s) return;
          if (track.kind === "audio") finish("mic-lost", endSignal(s));
          else setState((cur) => ({ ...cur, cameraEnabled: false }));
        };
        track.addEventListener("ended", onEnded);
        s.trackCleanups.push(() => track.removeEventListener("ended", onEnded));
      }
    },
    [endSignal, finish],
  );

  const flushCandidates = async (s: Session) => {
    const queued = s.pendingCandidates.splice(0);
    for (const c of queued) {
      try {
        await s.pc?.addIceCandidate(c);
      } catch {
        // A single bad candidate must not fail the call.
      }
    }
  };

  const createPeer = useCallback(
    (s: Session, iceServers: RTCIceServer[]) => {
      const pc = new RTCPeerConnection({ iceServers });
      s.pc = pc;
      s.local?.getTracks().forEach((track) => pc.addTrack(track, s.local as MediaStream));
      pc.onicecandidate = (e) => {
        if (e.candidate && s.callId && sessionRef.current === s) {
          signal({ type: "call.ice_candidate", data: { callId: s.callId, candidate: e.candidate.toJSON() } });
        }
      };
      pc.ontrack = (e) => {
        if (sessionRef.current !== s) return;
        const stream = e.streams[0] ?? s.remote ?? new MediaStream();
        if (!e.streams[0]) stream.addTrack(e.track);
        s.remote = stream;
        setState((cur) => ({ ...cur, remoteStream: stream }));
      };
      pc.onconnectionstatechange = () => {
        if (sessionRef.current !== s) return;
        switch (pc.connectionState) {
          case "connected":
            if (s.connectTimer) {
              clearTimeout(s.connectTimer);
              s.connectTimer = null;
            }
            setState((cur) => ({
              ...cur,
              status: "active",
              reconnecting: false,
              startedAt: cur.startedAt ?? Date.now(),
            }));
            break;
          case "disconnected":
            setState((cur) => ({ ...cur, reconnecting: true }));
            break;
          case "failed":
            finish("failed", endSignal(s));
            break;
        }
      };
      return pc;
    },
    [endSignal, finish, signal],
  );

  const startConnectTimer = useCallback(
    (s: Session) => {
      if (s.connectTimer) clearTimeout(s.connectTimer);
      s.connectTimer = setTimeout(() => {
        if (sessionRef.current === s && stateRef.current.status !== "active") {
          finish("failed", endSignal(s));
        }
      }, CONNECT_TIMEOUT_MS);
    },
    [endSignal, finish],
  );

  const startCall = useCallback<CallContextValue["startCall"]>(
    (target, type) => {
      const cur = stateRef.current.status;
      // A call is genuinely in progress: its overlay is on screen and the
      // header buttons are disabled, so there is nothing else to start.
      if (cur !== "idle" && cur !== "ended") return;
      // A leftover session while idle would otherwise swallow every click.
      if (sessionRef.current) teardown();
      clearEndedSummary();
      const token = ++tokenRef.current;
      const s: Session = {
        token,
        callId: null,
        type,
        pc: null,
        local: null,
        remote: null,
        offerSdp: null,
        remoteReady: false,
        pendingCandidates: [],
        ringTimer: null,
        connectTimer: null,
        trackCleanups: [],
      };
      sessionRef.current = s;
      setState({
        ...IDLE,
        status: "outgoing",
        direction: "outgoing",
        conversationId: target.conversationId,
        peer: target.peer,
        type,
        cameraEnabled: type === "video",
      });
      if (!isSupported()) {
        finish("unsupported");
        return;
      }

      void (async () => {
        // Readiness is checked on every click with a fresh request (never a
        // cached earlier failure). Every failure ends in a visible reason.
        try {
          const config = await getCallConfig();
          if (tokenRef.current !== token) return;
          iceServersRef.current = config.iceServers;
        } catch (err) {
          if (tokenRef.current !== token) return;
          finish(
            err instanceof ApiError && (err.status === 404 || err.status === 405 || err.status === 501)
              ? "unavailable"
              : "failed",
          );
          return;
        }

        let media: MediaStream;
        try {
          media = await acquireMedia(type);
        } catch (err) {
          if (tokenRef.current === token) finish(mediaErrorReason(err, type));
          return;
        }
        if (tokenRef.current !== token) {
          media.getTracks().forEach((t) => t.stop()); // cancelled during the prompt
          return;
        }
        s.local = media;
        watchLocalTracks(s, media);
        setState((c) => ({ ...c, localStream: media }));

        try {
          const created = await createCall({ conversationId: target.conversationId, type });
          if (tokenRef.current !== token) {
            // Cancelled while the server created the call: tell the callee.
            signal({ type: "call.cancel", data: { callId: created.callId, reason: "cancelled" } });
            return;
          }
          s.callId = created.callId;
          setState((c) => ({ ...c, callId: created.callId }));
          const pc = createPeer(s, created.iceServers ?? iceServersRef.current);
          const offer = await pc.createOffer();
          await pc.setLocalDescription(offer);
          if (tokenRef.current !== token) return;
          if (!offer.sdp || !signal({ type: "call.offer", data: { callId: created.callId, sdp: offer.sdp } })) {
            finish("failed");
            return;
          }
          s.ringTimer = setTimeout(() => {
            if (sessionRef.current === s && stateRef.current.status === "outgoing") {
              finish("timeout-outgoing", { type: "call.cancel", data: { callId: created.callId, reason: "timeout" } });
            }
          }, RING_TIMEOUT_MS);
        } catch (err) {
          if (tokenRef.current !== token) return;
          finish(err instanceof ApiError && err.status === 409 ? "busy" : "failed", endSignal(s));
        }
      })();
    },
    [createPeer, endSignal, finish, signal, teardown, watchLocalTracks],
  );

  const accept = useCallback(() => {
    const s = sessionRef.current;
    if (!s || stateRef.current.status !== "incoming" || !s.callId || !s.offerSdp) return;
    const callId = s.callId;
    const offerSdp = s.offerSdp;
    const token = s.token;
    if (s.ringTimer) {
      clearTimeout(s.ringTimer);
      s.ringTimer = null;
    }
    // The connect timer starts once the answer is sent, so a slow permission
    // prompt cannot fail the call; the caller's ring timeout still bounds it.
    setState((c) => ({ ...c, status: "connecting" }));

    void (async () => {
      if (!isSupported()) {
        finish("unsupported", { type: "call.reject", data: { callId, reason: "media_unavailable" } });
        return;
      }
      let media: MediaStream;
      try {
        media = await acquireMedia(s.type);
      } catch (err) {
        if (tokenRef.current === token) {
          finish(mediaErrorReason(err, s.type), { type: "call.reject", data: { callId, reason: "media_unavailable" } });
        }
        return;
      }
      if (tokenRef.current !== token) {
        media.getTracks().forEach((t) => t.stop());
        return;
      }
      s.local = media;
      watchLocalTracks(s, media);
      setState((c) => ({ ...c, localStream: media }));
      try {
        const config = await getCallConfig().catch(() => null);
        if (tokenRef.current !== token) return;
        if (config) iceServersRef.current = config.iceServers;
        const pc = createPeer(s, iceServersRef.current);
        await pc.setRemoteDescription({ type: "offer", sdp: offerSdp });
        s.remoteReady = true;
        await flushCandidates(s);
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        if (tokenRef.current !== token) return;
        if (!answer.sdp || !signal({ type: "call.answer", data: { callId, sdp: answer.sdp } })) {
          finish("failed", endSignal(s));
          return;
        }
        startConnectTimer(s);
      } catch {
        if (tokenRef.current === token) finish("failed", endSignal(s));
      }
    })();
  }, [createPeer, endSignal, finish, signal, startConnectTimer, watchLocalTracks]);

  const decline = useCallback(() => {
    const s = sessionRef.current;
    if (!s || stateRef.current.status !== "incoming" || !s.callId) return;
    finish("rejected", { type: "call.reject", data: { callId: s.callId, reason: "declined" } });
  }, [finish]);

  const hangUp = useCallback(() => {
    const s = sessionRef.current;
    if (!s) return;
    const status = stateRef.current.status;
    if (status === "incoming") {
      decline();
    } else if (status === "outgoing") {
      finish("cancelled", s.callId ? { type: "call.cancel", data: { callId: s.callId, reason: "cancelled" } } : undefined);
    } else {
      finish("ended", endSignal(s));
    }
  }, [decline, endSignal, finish]);

  const toggleMute = useCallback(() => {
    const s = sessionRef.current;
    if (!s?.local) return;
    const muted = !stateRef.current.muted;
    s.local.getAudioTracks().forEach((t) => (t.enabled = !muted));
    setState((c) => ({ ...c, muted }));
  }, []);

  const toggleCamera = useCallback(() => {
    const s = sessionRef.current;
    if (!s?.local || s.type !== "video") return;
    const tracks = s.local.getVideoTracks().filter((t) => t.readyState === "live");
    if (tracks.length === 0) return;
    const enabled = !stateRef.current.cameraEnabled;
    tracks.forEach((t) => (t.enabled = enabled));
    setState((c) => ({ ...c, cameraEnabled: enabled }));
  }, []);

  const dismiss = useCallback(() => {
    if (stateRef.current.status !== "ended") return;
    clearEndedSummary();
    setState(IDLE);
  }, []);

  // Incoming signaling on the existing socket.
  useEffect(() => {
    const onSignal = (event: CallSignalEvent) => {
      const d = event.data;
      const callId = str(d.callId);
      if (!callId) return;
      const s = sessionRef.current;

      if (event.type === "call.offer") {
        const sdp = str(d.sdp);
        const conversationId = str(d.conversationId);
        const callType = d.callType === "video" ? "video" : d.callType === "voice" ? "voice" : null;
        const caller = d.caller as Record<string, unknown> | null;
        const peer: CallPeer | null =
          caller && str(caller.id) && str(caller.username) && str(caller.displayName)
            ? {
                id: caller.id as string,
                username: caller.username as string,
                displayName: caller.displayName as string,
                avatarUrl: str(caller.avatarUrl),
              }
            : null;
        if (!sdp || !conversationId || !callType || !peer) return;
        if (s) {
          // One call per client: anyone else ringing gets "busy".
          if (s.callId !== callId) signal({ type: "call.busy", data: { callId } });
          return;
        }
        clearEndedSummary();
        const next: Session = {
          token: ++tokenRef.current,
          callId,
          type: callType,
          pc: null,
          local: null,
          remote: null,
          offerSdp: sdp,
          remoteReady: false,
          pendingCandidates: [],
          ringTimer: null,
          connectTimer: null,
          trackCleanups: [],
        };
        sessionRef.current = next;
        next.ringTimer = setTimeout(() => {
          if (sessionRef.current === next && stateRef.current.status === "incoming") {
            finish("timeout-incoming", { type: "call.reject", data: { callId, reason: "timeout" } });
          }
        }, RING_TIMEOUT_MS);
        setState({
          ...IDLE,
          status: "incoming",
          direction: "incoming",
          callId,
          conversationId,
          peer,
          type: callType,
          cameraEnabled: callType === "video",
        });
        return;
      }

      // Everything else must belong to the current call.
      if (!s || s.callId !== callId) return;
      switch (event.type) {
        case "call.answer": {
          const sdp = str(d.sdp);
          if (!sdp || !s.pc || stateRef.current.status !== "outgoing") return;
          if (s.ringTimer) {
            clearTimeout(s.ringTimer);
            s.ringTimer = null;
          }
          setState((c) => ({ ...c, status: "connecting" }));
          startConnectTimer(s);
          const pc = s.pc;
          void pc
            .setRemoteDescription({ type: "answer", sdp })
            .then(() => {
              s.remoteReady = true;
              return flushCandidates(s);
            })
            .catch(() => {
              if (sessionRef.current === s) finish("failed", endSignal(s));
            });
          return;
        }
        case "call.ice_candidate": {
          const candidate = d.candidate as RTCIceCandidateInit | null;
          if (!candidate || typeof candidate !== "object") return;
          if (s.pc && s.remoteReady) {
            s.pc.addIceCandidate(candidate).catch(() => {
              // Ignore individual candidate failures.
            });
          } else {
            s.pendingCandidates.push(candidate);
          }
          return;
        }
        case "call.reject":
          finish("rejected");
          return;
        case "call.busy":
          finish("busy");
          return;
        case "call.cancel":
          finish("cancelled");
          return;
        case "call.end":
          finish("ended");
          return;
      }
    };
    return subscribeCallSignal(onSignal);
  }, [endSignal, finish, signal, startConnectTimer, subscribeCallSignal]);

  // Unmount (logout / leaving the app shell): end any call and free media.
  useEffect(
    () => () => {
      const s = sessionRef.current;
      if (s?.callId) {
        const status = stateRef.current.status;
        if (status === "incoming") signal({ type: "call.reject", data: { callId: s.callId, reason: "declined" } });
        else if (status === "outgoing") signal({ type: "call.cancel", data: { callId: s.callId, reason: "cancelled" } });
        else signal({ type: "call.end", data: { callId: s.callId } });
      }
      teardown();
      if (endedTimerRef.current) clearTimeout(endedTimerRef.current);
    },
    [signal, teardown],
  );

  const value = useMemo<CallContextValue>(
    () => ({ state, startCall, accept, decline, hangUp, toggleMute, toggleCamera, dismiss }),
    [state, startCall, accept, decline, hangUp, toggleMute, toggleCamera, dismiss],
  );

  return <CallContext.Provider value={value}>{children}</CallContext.Provider>;
}

export function useCalls(): CallContextValue {
  const ctx = useContext(CallContext);
  if (!ctx) throw new Error("useCalls must be used within a CallProvider");
  return ctx;
}

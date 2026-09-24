/**
 * Calls V1 — shared types and the PROPOSED signaling contract.
 *
 * 1-to-1 voice/video calls in direct conversations only. Signaling reuses the
 * existing app WebSocket (see realtime-context); REST is used only for the
 * authorization-heavy "start a call" step and for ICE configuration. Every
 * event name and payload shape the frontend relies on lives in this file.
 */

export type CallType = "voice" | "video";
export type CallDirection = "incoming" | "outgoing";

export type CallStatus =
  | "idle"
  | "outgoing" // caller: acquiring media / ringing the callee
  | "incoming" // callee: ringing, awaiting accept/decline
  | "connecting" // SDP exchanged, ICE/DTLS in progress
  | "active" // media flowing
  | "ended"; // terminal; auto-returns to idle

/** Why a call ended — drives the localized summary shown briefly. */
export type CallEndReason =
  | "ended" // normal hang-up by either side
  | "rejected" // callee declined
  | "busy" // callee already in a call
  | "cancelled" // caller cancelled before answer
  | "timeout-outgoing" // no answer
  | "timeout-incoming" // missed
  | "failed" // signaling/ICE/peer failure
  | "mic-denied"
  | "camera-denied"
  | "no-mic"
  | "no-camera"
  | "unsupported"
  | "unavailable" // calls config endpoint missing on this backend
  | "mic-lost";

/** Identity of the other participant (from real API data, never invented). */
export interface CallPeer {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

/* ---------------------------- REST boundary ---------------------------- */

/** GET /api/v1/calls/config — also the "calls are available" capability. */
export interface CallConfig {
  iceServers: RTCIceServer[];
}

/** POST /api/v1/calls request. The callee is derived server-side. */
export interface CreateCallRequest {
  conversationId: string;
  type: CallType;
}

/** POST /api/v1/calls response (201). */
export interface CreateCallResponse {
  callId: string;
  /** Optional per-call ICE servers (e.g. short-lived TURN credentials). */
  iceServers?: RTCIceServer[];
}

/* ------------------------ WebSocket signaling -------------------------- */

/** Client → server frames, sent on the existing socket as {type, data}. */
export type OutgoingSignal =
  | { type: "call.offer"; data: { callId: string; sdp: string } }
  | { type: "call.answer"; data: { callId: string; sdp: string } }
  | { type: "call.ice_candidate"; data: { callId: string; candidate: RTCIceCandidateInit } }
  | { type: "call.reject"; data: { callId: string; reason: "declined" | "timeout" | "media_unavailable" } }
  | { type: "call.cancel"; data: { callId: string; reason: "cancelled" | "timeout" } }
  | { type: "call.end"; data: { callId: string } }
  | { type: "call.busy"; data: { callId: string } };

/** Server → client frames. The server relays only between the call's two participants. */
export type IncomingSignal =
  | {
      type: "call.offer";
      data: {
        callId: string;
        conversationId: string;
        callType: CallType;
        caller: CallPeer;
        sdp: string;
      };
    }
  | { type: "call.answer"; data: { callId: string; sdp: string } }
  | { type: "call.ice_candidate"; data: { callId: string; candidate: RTCIceCandidateInit } }
  | { type: "call.reject"; data: { callId: string } }
  | { type: "call.cancel"; data: { callId: string } }
  | { type: "call.end"; data: { callId: string } }
  | { type: "call.busy"; data: { callId: string } };

export const CALL_SIGNAL_TYPES = [
  "call.offer",
  "call.answer",
  "call.ice_candidate",
  "call.reject",
  "call.cancel",
  "call.end",
  "call.busy",
] as const;

/** How long a call may ring before it is treated as unanswered. */
export const RING_TIMEOUT_MS = 45_000;
/** How long SDP/ICE may take before a connecting call is failed. */
export const CONNECT_TIMEOUT_MS = 30_000;
/** How long the ended summary stays before returning to idle. */
export const ENDED_DISPLAY_MS = 2_500;

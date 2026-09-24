/**
 * REST boundary for Calls V1 (PROPOSED — the backend does not expose these
 * yet). All call endpoint paths live here. Until they exist, getCallConfig
 * rejects and the UI keeps call entry points disabled; nothing is simulated.
 */

import { apiFetch } from "@/lib/api";
import type { CallConfig, CreateCallRequest, CreateCallResponse } from "./call-types";

/** ICE servers for this user; its success is also the calls capability check. */
export function getCallConfig(signal?: AbortSignal): Promise<CallConfig> {
  return apiFetch<CallConfig>("/api/v1/calls/config", { signal }).then((c) => ({
    iceServers: Array.isArray(c?.iceServers) ? c.iceServers : [],
  }));
}

/**
 * Ask the server to start a call in a direct conversation. The server
 * authorizes caller/callee (participants, blocks, busy) and returns a callId;
 * the SDP offer is then sent over the WebSocket with that id.
 */
export function createCall(input: CreateCallRequest): Promise<CreateCallResponse> {
  return apiFetch<CreateCallResponse>("/api/v1/calls", { method: "POST", body: input });
}

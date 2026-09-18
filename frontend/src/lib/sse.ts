import { useAuthStore } from "@/store/auth.store";
import type { SSEEvent } from "@/types";

type Listener<T = unknown> = (event: SSEEvent<T>) => void;

// Named SSE events the server emits with `event: <type>` (not default messages).
const CORE_EVENTS = [
  "connected",
  "notification.created",
  "notification.read",
] as const;

/**
 * Connects to /api/events and fans out typed events to subscribers.
 * EventSource cannot set Authorization — the token goes in the query string.
 * Named events must use addEventListener; onmessage only gets unnamed ones.
 */
class SSEClient {
  private es: EventSource | null = null;
  private listeners = new Map<string, Set<Listener>>();
  private boundTypes = new Set<string>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectDelay = 1_000;
  private maxDelay = 30_000;
  private intentionalClose = false;

  connect() {
    if (this.es) return;

    const { accessToken } = useAuthStore.getState();
    if (!accessToken) return;

    this.intentionalClose = false;
    const url = `/api/events?token=${encodeURIComponent(accessToken)}`;
    this.es = new EventSource(url);
    this.boundTypes.clear();

    this.es.onopen = () => {
      this.reconnectDelay = 1_000;
    };

    // Bind known + currently subscribed named events.
    for (const type of CORE_EVENTS) this.bindType(type);
    for (const type of this.listeners.keys()) this.bindType(type);

    // Unnamed / default messages (if any).
    this.es.onmessage = (raw) => this.handleRaw(raw);

    this.es.onerror = () => {
      this.es?.close();
      this.es = null;
      this.boundTypes.clear();
      if (this.intentionalClose) return;
      this.scheduleReconnect();
    };
  }

  /** Close and open again with the current access token (e.g. after refresh). */
  reconnect() {
    this.disconnect();
    this.connect();
  }

  disconnect() {
    this.intentionalClose = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.es?.close();
    this.es = null;
    this.boundTypes.clear();
  }

  on<T = unknown>(eventType: string, listener: Listener<T>): () => void {
    if (!this.listeners.has(eventType)) {
      this.listeners.set(eventType, new Set());
    }
    this.listeners.get(eventType)!.add(listener as Listener);
    this.bindType(eventType);

    return () => {
      this.listeners.get(eventType)?.delete(listener as Listener);
    };
  }

  private scheduleReconnect() {
    if (this.reconnectTimer) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.reconnectDelay);
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxDelay);
  }

  private bindType(type: string) {
    if (!this.es || this.boundTypes.has(type) || type === "*") return;
    this.boundTypes.add(type);
    this.es.addEventListener(type, (raw) => this.handleRaw(raw as MessageEvent, type));
  }

  private handleRaw(raw: MessageEvent, forcedType?: string) {
    try {
      const data = JSON.parse(raw.data as string) as Record<string, unknown>;
      const type =
        forcedType ??
        (typeof data.type === "string" ? data.type : "message");
      const event = { ...data, type } as SSEEvent;
      this.dispatch(type, event);
      this.dispatch("*", event);
    } catch {
      // ignore malformed / keepalive noise
    }
  }

  private dispatch(type: string, event: SSEEvent) {
    this.listeners.get(type)?.forEach((fn) => fn(event));
  }
}

export const sseClient = new SSEClient();

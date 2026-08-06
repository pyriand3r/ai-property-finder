import { useMessage } from 'naive-ui';
import { onBeforeUnmount, onMounted } from 'vue';

/** Payload shape for listing.rejection_analyzed (backend/internal/service
 * /rejection_analysis_service.go AnalyzeRejection — spec 01-04 Layer 8.1). */
interface RejectionAnalyzedPayload {
  profile_id: string;
  listing_id: string;
  message_id: string;
}

function parsePayload(event: Event): RejectionAnalyzedPayload | null {
  const messageEvent = event as MessageEvent<string>;
  try {
    return JSON.parse(messageEvent.data) as RejectionAnalyzedPayload;
  } catch {
    return null;
  }
}

/** Toasts "Rejection verarbeitet — Vorschläge im Chat verfügbar." whenever
 * the backend's async rejection analysis (Layer 8.1) produces at least one
 * propose_criteria/update_context proposal for this profile — regardless
 * of which tab (Chat, Inserate, ...) is currently open, since the trigger
 * (setting a Listing to rejected) usually happens from the Inserate tab,
 * not Chat. Mounted once at the ProfileShellView level so it survives tab
 * switches instead of per-tab like useReevaluationBanner.ts (that one is
 * fine to duplicate per List-View mount; this one specifically must not
 * miss the event because Chat wasn't the active tab). */
export function useRejectionAnalysisToast(profileId: string) {
  const message = useMessage();
  let eventSource: EventSource | null = null;

  function connect() {
    eventSource = new EventSource('/api/events');
    eventSource.addEventListener('listing.rejection_analyzed', (event) => {
      const payload = parsePayload(event);
      if (payload && payload.profile_id === profileId) {
        message.success('Rejection verarbeitet — Vorschläge im Chat verfügbar.');
      }
    });
  }

  onMounted(connect);
  onBeforeUnmount(() => eventSource?.close());
}

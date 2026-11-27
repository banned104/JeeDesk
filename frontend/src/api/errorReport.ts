import type { App } from 'vue';
import { bridge, lastBindingCall } from './bridge';

// The WebView is a second run-time that dies quietly: a failed render removes the subtree, an
// unhandled rejection disappears into the console, and a GUI build has no console to look at. These
// hooks turn every one of them into a crash report next to the Go ones.

const MAX_PER_SIGNATURE = 3;
const MAX_REPORTS = 20;
const seen = new Map<string, number>();
let reported = 0;

function describe(value: unknown): { message: string; stack: string } {
  if (value instanceof Error) {
    return { message: value.message || value.name, stack: value.stack ?? '' };
  }
  if (typeof value === 'string') return { message: value, stack: '' };
  try {
    return { message: JSON.stringify(value), stack: '' };
  } catch {
    return { message: String(value), stack: '' };
  }
}

/** reportError writes one report, rate limited so a broken render loop cannot fill the disk. */
function reportError(kind: string, value: unknown, detail: string): void {
  if (reported >= MAX_REPORTS) return;
  const { message, stack } = describe(value);
  const signature = `${kind}\u0000${message.slice(0, 160)}`;
  const count = seen.get(signature) ?? 0;
  if (count >= MAX_PER_SIGNATURE) return;
  seen.set(signature, count + 1);
  reported += 1;

  const binding = lastBindingCall();
  const where = [detail, binding ? `last binding: ${binding}` : ''].filter(Boolean).join(' | ');
  try {
    bridge.reportFrontendError(kind, message, stack, where);
  } catch {
    // Reporting must never throw, and there is nowhere else to write from here.
  }
}

/**
 * installErrorReporting hooks the three places a WebView failure surfaces. Call it before mounting
 * so a failure in the first render is caught too.
 */
export function installErrorReporting(app: App): void {
  app.config.errorHandler = (err, instance, info) => {
    const component = instance?.$options?.__name ?? instance?.$options?.name ?? '';
    reportError('vue', err, `${info}${component ? ` in ${component}` : ''}`);
  };

  window.addEventListener(
    'error',
    (event) => {
      // Failed <img>/<link> loads bubble here too; they are not crashes.
      const target = event.target as HTMLElement | null;
      if (target && target !== (window as unknown as HTMLElement)) return;
      reportError('window', event.error ?? event.message, `${event.filename}:${event.lineno}:${event.colno}`);
    },
    true
  );

  window.addEventListener('unhandledrejection', (event) => {
    reportError('promise', event.reason, '');
  });
}

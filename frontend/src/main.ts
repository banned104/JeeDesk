import { createApp } from 'vue';
import './style.css';
import App from './App.vue';
import { installErrorReporting } from './api/errorReport';

// Block WebView2's default handling of dragged files/folders (navigating to the
// file, or opening a preview window). Actual file intake is done by the Wails
// file-drop pipeline on elements carrying the --wails-drop-target CSS property;
// preventDefault here only suppresses the browser default and keeps other
// listeners (the Wails runtime) working.
for (const type of ['dragover', 'drop'] as const) {
  document.addEventListener(
    type,
    (event) => {
      const dragEvent = event as DragEvent;
      if (dragEvent.dataTransfer?.types.includes('Files')) {
        dragEvent.preventDefault();
      }
    },
    true
  );
}

const app = createApp(App);
// Before mount: a failure in the first render must already be reportable.
installErrorReporting(app);
app.mount('#app');

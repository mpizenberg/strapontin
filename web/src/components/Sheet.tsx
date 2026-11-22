import type { JSX } from "@solidjs/web";

/** Bottom sheet. Render it only while open. */
export function Sheet(props: { title: string; onClose: () => void; children: JSX.Element }) {
  return (
    <div class="sheet" role="dialog" aria-modal="true" aria-label={props.title}>
      <button class="backdrop" aria-label="Fermer" onClick={() => props.onClose()} />
      <div class="panel">
        <span class="grabber" />
        <div class="sheet-head">
          <h2>{props.title}</h2>
          <button class="icon-btn" aria-label="Fermer" onClick={() => props.onClose()}>×</button>
        </div>
        {props.children}
      </div>
    </div>
  );
}

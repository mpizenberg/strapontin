import { createSignal, Show } from "solid-js";
import { formatDay } from "../format";
import type { InviteLink } from "../types";

/** A one-time link to send through any messaging app. */
export function ShareLink(props: { link: InviteLink; text: string }) {
  const [copied, setCopied] = createSignal(false);
  const canShare = typeof navigator.share === "function";
  const share = () => navigator.share({ title: "Strapontin", text: props.text, url: props.link.url }).catch(() => {});
  const copy = async () => {
    await navigator.clipboard.writeText(props.link.url);
    setCopied(true);
  };
  return (
    <div class="stack">
      <input class="input" readonly value={props.link.url} aria-label="Lien" onFocus={(e) => e.currentTarget.select()} />
      <p class="meta">Valable jusqu'au {formatDay(props.link.expires_at)}, pour une seule personne.</p>
      <div class="btn-row">
        <Show when={canShare}>
          <button class="btn primary" onClick={share}>Partager</button>
        </Show>
        <button class="btn" onClick={copy}>{copied() ? "Copié" : "Copier"}</button>
      </div>
    </div>
  );
}

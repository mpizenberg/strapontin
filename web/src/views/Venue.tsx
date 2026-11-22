import { createMemo, createSignal, For, refresh, Show } from "solid-js";
import { api } from "../api";
import { back } from "../router";
import { aria, plural } from "../format";
import { NoteField } from "../components/NoteField";
import { Sheet } from "../components/Sheet";
import { SummaryRow } from "../components/SummaryRow";

export function VenueView(props: { id: number }) {
  const v = createMemo(() => api.venue(props.id));
  const [editing, setEditing] = createSignal(false);

  const toggleFavorite = async () => {
    await api.setFavorite("venue", props.id, !v().favorite);
    refresh(v);
  };

  return (
    <>
      <header class="appbar">
        <button class="appbar-back link" aria-label="Retour" onClick={() => back()}>‹</button>
        <h1>Lieu</h1>
        <button class="btn ghost small" onClick={() => setEditing(true)}>Modifier</button>
      </header>
      <main class="page">
        <div class="section">
          <h2 class="title">{v().name}</h2>
          <p class="meta">{[v().address, v().city].filter(Boolean).join(", ")}</p>
          <div class="btn-row">
            <button class="btn" aria-pressed={aria(v().favorite)} onClick={toggleFavorite}>
              <span class="star">{v().favorite ? "★" : "☆"}</span> Favori
            </button>
          </div>
        </div>

        <section class="section">
          <h2 class="section-title">Ma note</h2>
          <NoteField entity="venue" id={props.id} body={v().note} />
        </section>

        <section class="section">
          <h2 class="section-title">{plural(v().productions.length, "spectacle", "spectacles")}</h2>
          <ul class="list">
            <For each={v().productions}>{(s) => <li><SummaryRow summary={s} /></li>}</For>
          </ul>
        </section>
      </main>

      <Show when={editing()}>
        <VenueSheet venue={v()} onClose={() => setEditing(false)} onSave={async (fields) => {
          await api.updateVenue(props.id, fields);
          setEditing(false);
          refresh(v);
        }} />
      </Show>
    </>
  );
}

type Fields = { name: string; city: string; address: string };

function VenueSheet(props: { venue: Fields; onClose: () => void; onSave: (v: Fields) => Promise<void> }) {
  const [name, setName] = createSignal(() => props.venue.name);
  const [city, setCity] = createSignal(() => props.venue.city);
  const [address, setAddress] = createSignal(() => props.venue.address);
  return (
    <Sheet title="Modifier le lieu" onClose={props.onClose}>
      <label class="field"><span class="label">Nom</span>
        <input class="input" value={name()} onInput={(e) => setName(e.currentTarget.value)} /></label>
      <label class="field"><span class="label">Adresse</span>
        <input class="input" value={address()} onInput={(e) => setAddress(e.currentTarget.value)} /></label>
      <label class="field"><span class="label">Ville</span>
        <input class="input" value={city()} onInput={(e) => setCity(e.currentTarget.value)} /></label>
      <button class="btn primary block" onClick={() => props.onSave({ name: name(), city: city(), address: address() })}>Enregistrer</button>
    </Sheet>
  );
}

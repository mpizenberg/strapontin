import { createMemo, createSignal, For, refresh, Show } from "solid-js";
import { api } from "../api";
import { back } from "../router";
import { aria, capitalize, plural, year } from "../format";
import { roleLabel, roles } from "../roles";
import { NoteField } from "../components/NoteField";
import { Sheet } from "../components/Sheet";
import { SummaryRow } from "../components/SummaryRow";
import type { Person } from "../types";

function groupByRole(p: Person) {
  return roles
    .map((r) => ({ label: r.label, credits: p.credits.filter((c) => c.role === r.key) }))
    .filter((g) => g.credits.length > 0);
}

export function PersonView(props: { id: number }) {
  const p = createMemo(() => api.person(props.id));
  const [renaming, setRenaming] = createSignal(false);
  const productions = () => new Set(p().credits.map((c) => c.production.id)).size;
  const roleSummary = () => capitalize([...new Set(p().credits.map((c) => roleLabel(c.role)))].join(", "));
  const wikidata = () => p().external_ids.find((e) => e.scheme === "wikidata")?.value;

  const toggleFavorite = async () => {
    await api.setFavorite("person", props.id, !p().favorite);
    refresh(p);
  };

  return (
    <>
      <header class="appbar">
        <button class="appbar-back link" aria-label="Retour" onClick={() => back()}>‹</button>
        <h1>Personne</h1>
        <button class="btn ghost small" onClick={() => setRenaming(true)}>Renommer</button>
      </header>
      <main class="page">
        <div class="section">
          <h2 class="title">{p().name}</h2>
          <p class="meta">{[roleSummary(), plural(productions(), "spectacle", "spectacles")].filter(Boolean).join(" · ")}</p>
          <div class="btn-row">
            <button class="btn" aria-pressed={aria(p().favorite)} onClick={toggleFavorite}>
              <span class="star">{p().favorite ? "★" : "☆"}</span> Favori
            </button>
            <Show when={wikidata()}>
              {(q) => <a class="btn" href={`https://www.wikidata.org/wiki/${q()}`} target="_blank" rel="noopener noreferrer">Wikidata ↗</a>}
            </Show>
          </div>
        </div>

        <section class="section">
          <h2 class="section-title">Ma note</h2>
          <NoteField entity="person" id={props.id} body={p().note} />
        </section>

        <For each={groupByRole(p())}>
          {(g) => (
            <section class="section">
              <h2 class="section-title">{g.label}</h2>
              <ul class="list">
                <For each={g.credits}>
                  {(c) => (
                    <li>
                      <SummaryRow summary={c.production}
                        meta={[c.character, c.production.venue?.name, year(c.production.ends_on ?? c.production.starts_on)].filter(Boolean).join(" · ")} />
                    </li>
                  )}
                </For>
              </ul>
            </section>
          )}
        </For>
      </main>

      <Show when={renaming()}>
        <RenameSheet name={p().name} onClose={() => setRenaming(false)} onSave={async (name) => {
          await api.renamePerson(props.id, name);
          setRenaming(false);
          refresh(p);
        }} />
      </Show>
    </>
  );
}

function RenameSheet(props: { name: string; onClose: () => void; onSave: (name: string) => Promise<void> }) {
  const [name, setName] = createSignal(() => props.name);
  return (
    <Sheet title="Renommer" onClose={props.onClose}>
      <p class="meta">Corrige le nom sur toutes les fiches où la personne apparaît.</p>
      <input class="input" aria-label="Nom" value={name()} onInput={(e) => setName(e.currentTarget.value)} />
      <button class="btn primary block" onClick={() => props.onSave(name())}>Enregistrer</button>
    </Sheet>
  );
}

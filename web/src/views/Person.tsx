import { createMemo, createSignal, Errored, For, Loading, refresh, Show } from "solid-js";
import { api } from "../api";
import { back } from "../router";
import { aria, capitalize, plural, year } from "../format";
import { roleLabel, roles } from "../roles";
import { NoteField } from "../components/NoteField";
import { Sheet } from "../components/Sheet";
import { SummaryRow } from "../components/SummaryRow";
import type { Homonym, Person } from "../types";

function groupByRole(p: Person) {
  return roles
    .map((r) => ({ label: r.label, credits: p.credits.filter((c) => c.role === r.key) }))
    .filter((g) => g.credits.length > 0);
}

export function PersonView(props: { id: number }) {
  const p = createMemo(() => api.person(props.id));
  const [renaming, setRenaming] = createSignal(false);
  const [linking, setLinking] = createSignal(false);
  const [merging, setMerging] = createSignal<Homonym | null>(null);
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
            <Show when={wikidata()} fallback={<button class="btn" onClick={() => setLinking(true)}>Lier à Wikidata</button>}>
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

        <Show when={p().homonyms.length > 0}>
          <section class="section">
            <h2 class="section-title">Autres fiches au même nom</h2>
            <p class="meta">S'il s'agit de la même personne, fusionnez les fiches.</p>
            <ul class="list">
              <For each={p().homonyms}>
                {(h) => (
                  <li class="row">
                    <a class="grow" href={`/personnes/${h.id}`}>
                      <span class="name">{h.name}</span>
                      <br />
                      <span class="meta">{homonymMeta(h)}</span>
                    </a>
                    <button class="btn small" onClick={() => setMerging(h)}>Fusionner</button>
                  </li>
                )}
              </For>
            </ul>
          </section>
        </Show>
      </main>

      <Show when={linking()}>
        <WikidataSheet name={p().name} onClose={() => setLinking(false)} onPick={async (id) => {
          await api.linkWikidata(props.id, id);
          setLinking(false);
          refresh(p);
        }} />
      </Show>
      <Show when={merging()}>
        {(h) => (
          <MergeSheet homonym={h()} onClose={() => setMerging(null)} onMerge={async () => {
            await api.mergePerson(props.id, h().id);
            setMerging(null);
            refresh(p);
          }} />
        )}
      </Show>
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

function homonymMeta(h: Homonym) {
  return [capitalize(h.roles.map(roleLabel).join(", ")), plural(h.productions, "spectacle", "spectacles")].filter(Boolean).join(" · ");
}

function MergeSheet(props: { homonym: Homonym; onClose: () => void; onMerge: () => Promise<void> }) {
  const [error, setError] = createSignal("");
  const merge = async () => {
    try {
      await props.onMerge();
    } catch (err) {
      setError((err as Error).message);
    }
  };
  return (
    <Sheet title="Fusionner les fiches" onClose={props.onClose}>
      <p>
        L'autre fiche « {props.homonym.name} » rejoint celle-ci : ses crédits, favoris, notes et lien Wikidata y sont
        déplacés, puis elle est supprimée.
      </p>
      <p class="meta">À faire seulement s'il s'agit bien de la même personne : la fusion ne peut pas être annulée.</p>
      <Show when={error()}><p class="notice warn" role="alert">{error()}</p></Show>
      <button class="btn primary block" onClick={merge}>Fusionner</button>
    </Sheet>
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

/** Finds the Wikidata item for a person; the search runs once, with the name as typed. */
function WikidataSheet(props: { name: string; onClose: () => void; onPick: (id: string) => Promise<void> }) {
  const [q, setQ] = createSignal(() => props.name);
  const [asked, setAsked] = createSignal(() => props.name);
  const results = createMemo(() => api.wikidata(asked()));
  const [error, setError] = createSignal("");
  const pick = (id: string) => props.onPick(id).catch((e: Error) => setError(e.message));
  return (
    <Sheet title="Lier à Wikidata" onClose={props.onClose}>
      <form class="input-group" onSubmit={(e) => { e.preventDefault(); setAsked(q()); }}>
        <input class="input" aria-label="Nom" value={q()} onInput={(e) => setQ(e.currentTarget.value)} />
        <button class="btn" type="submit">Chercher</button>
      </form>
      <Show when={error()}><p class="notice warn">{error()}</p></Show>
      <Errored fallback={(err) => <p class="notice warn">{err() instanceof Error ? (err() as Error).message : String(err())}</p>}>
        <Loading fallback={<p class="loading">Recherche…</p>}>
          <Show when={results().length > 0} fallback={<p class="meta">Aucune entrée.</p>}>
            <ul class="list">
              <For each={results()}>
                {(e) => (
                  <li><button class="row" onClick={() => pick(e.id)}>
                    <span class="grow"><span class="name">{e.label}</span><Show when={e.description}><br /><span class="meta">{e.description}</span></Show></span>
                    <span class="meta">{e.id}</span>
                  </button></li>
                )}
              </For>
            </ul>
          </Show>
        </Loading>
      </Errored>
    </Sheet>
  );
}

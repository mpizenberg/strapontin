import { createMemo, createSignal, For, Loading, Show } from "solid-js";
import { api } from "../api";
import { navigate, route } from "../router";
import { SummaryRow } from "../components/SummaryRow";

/** The first web address in a text: a pasted link, or a link shared with some words around it. */
export function findURL(text: string): string | undefined {
  return text.match(/https?:\/\/\S+/i)?.[0] ?? (/^www\.\S+$/i.test(text.trim()) ? text.trim() : undefined);
}

const importPath = (url: string) => `/spectacles/nouveau?import=${encodeURIComponent(url)}`;

export function Chercher() {
  const [text, setText] = createSignal(() => route().query.get("q") ?? "");
  const query = createMemo(() => text().trim());
  const url = createMemo(() => findURL(query()));
  const results = createMemo(() => (query().length < 2 || url() ? null : api.search(query())));

  const paste = async () => {
    try {
      const t = await navigator.clipboard.readText();
      const u = findURL(t);
      if (u) navigate(importPath(u));
      else onInput(t);
    } catch {
      // Clipboard access refused: the member can still paste into the field.
    }
  };
  const empty = () => {
    const r = results();
    return !!r && r.productions.length + r.people.length + r.venues.length === 0;
  };

  const onInput = (value: string) => {
    setText(value);
    navigate(`/chercher${value.trim() ? `?q=${encodeURIComponent(value.trim())}` : ""}`, { replace: true });
  };

  return (
    <>
      <header class="appbar"><h1>Chercher</h1></header>
      <main class="page">
        <label class="search-field">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="6" /><path d="m20 20-4.5-4.5" /></svg>
          <input type="search" placeholder="Titre, artiste, lieu… ou lien" aria-label="Chercher" autocomplete="off"
            value={text()} onInput={(e) => onInput(e.currentTarget.value)} />
          <button class="btn ghost small" onClick={(e) => { e.preventDefault(); paste(); }}>Coller</button>
        </label>

        <Show when={url()}>
          {(u) => (
            <div class="card">
              <p><strong>Importer cette page ?</strong></p>
              <p class="meta">Strapontin lit la page du spectacle et vous propose une fiche à vérifier.</p>
              <a class="btn primary block" href={importPath(u())}>Lire la page</a>
            </div>
          )}
        </Show>

        <Loading fallback={<p class="loading">Recherche…</p>}>
          <Show when={results()}>
            {(r) => (
              <>
                <Show when={r().productions.length > 0}>
                  <section class="section">
                    <h2 class="section-title">Spectacles</h2>
                    <ul class="list">
                      <For each={r().productions}>{(s) => <li><SummaryRow summary={s} /></li>}</For>
                    </ul>
                  </section>
                </Show>
                <Show when={r().people.length > 0}>
                  <section class="section">
                    <h2 class="section-title">Personnes</h2>
                    <ul class="list">
                      <For each={r().people}>
                        {(p) => <li><a class="row" href={`/personnes/${p.id}`}><span class="grow name">{p.name}</span><span class="chev">›</span></a></li>}
                      </For>
                    </ul>
                  </section>
                </Show>
                <Show when={r().venues.length > 0}>
                  <section class="section">
                    <h2 class="section-title">Lieux</h2>
                    <ul class="list">
                      <For each={r().venues}>
                        {(v) => (
                          <li><a class="row" href={`/lieux/${v.id}`}>
                            <span class="grow"><span class="name">{v.name}</span><Show when={v.city}><br /><span class="meta">{v.city}</span></Show></span>
                            <span class="chev">›</span>
                          </a></li>
                        )}
                      </For>
                    </ul>
                  </section>
                </Show>
                <Show when={empty()}>
                  <p class="meta">Rien dans Strapontin pour « {query()} ».</p>
                </Show>
              </>
            )}
          </Show>
        </Loading>

        <a class="btn block" href={`/spectacles/nouveau${query() ? `?titre=${encodeURIComponent(query())}` : ""}`}>＋ Saisir un spectacle à la main</a>
      </main>
    </>
  );
}

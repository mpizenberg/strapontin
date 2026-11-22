import { createMemo, createSignal, For, Loading, Show } from "solid-js";
import { api } from "../api";
import { navigate, route } from "../router";
import { SummaryRow } from "../components/SummaryRow";
import { formatRange } from "../format";

/** The first web address in a text: a pasted link, or a link shared with some words around it. */
export function findURL(text: string): string | undefined {
  return text.match(/https?:\/\/\S+/i)?.[0] ?? (/^www\.\S+$/i.test(text.trim()) ? text.trim() : undefined);
}

const importPath = (url: string) => `/spectacles/nouveau?import=${encodeURIComponent(url)}`;

const agendaNames = { openagenda: "Agenda de la Métropole", tourisme: "Office de tourisme" } as const;
type Agenda = keyof typeof agendaNames;

export function Chercher() {
  const [text, setText] = createSignal(() => route().query.get("q") ?? "");
  const query = createMemo(() => text().trim());
  const url = createMemo(() => findURL(query()));
  const results = createMemo(() => (query().length < 2 || url() ? null : api.search(query())));
  // Outside agendas are only asked on submit: each request is a deliberate member action.
  const [asked, setAsked] = createSignal("");
  const agendas = (["openagenda", "tourisme"] as Agenda[]).map((source) => ({
    source,
    events: createMemo(() =>
      asked() && asked() === query()
        ? api.agendas(asked(), source).then((events) => ({ events, error: "" }), (e: Error) => ({ events: [], error: e.message }))
        : null,
    ),
  }));

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
        <form class="search-field" role="search" onSubmit={(e) => {
          e.preventDefault();
          if (url()) navigate(importPath(url()!));
          else if (query().length >= 2) setAsked(query());
        }}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="6" /><path d="m20 20-4.5-4.5" /></svg>
          <input type="search" enterkeyhint="search" placeholder="Titre, artiste, lieu… ou lien" aria-label="Chercher" autocomplete="off"
            value={text()} onInput={(e) => onInput(e.currentTarget.value)} />
          <button type="button" class="btn ghost small" onClick={paste}>Coller</button>
        </form>

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

        <Show when={query().length >= 2 && !url()}>
          <Show when={asked() === query()} fallback={
            <button class="btn block" onClick={() => setAsked(query())}>Chercher « {query()} » dans les agendas de Toulouse</button>
          }>
            <For each={agendas}>
              {(a) => (
                <Loading fallback={<p class="loading">{agendaNames[a.source]}…</p>}>
                  <Show when={a.events()}>
                    {(r) => (
                      <section class="section">
                        <h2 class="section-title">{agendaNames[a.source]}</h2>
                        <Show when={!r().error} fallback={<p class="notice warn">{r().error} La recherche dans Strapontin et l'import par lien restent possibles.</p>}>
                          <Show when={r().events.length > 0} fallback={<p class="meta">Rien trouvé.</p>}>
                            <ul class="list">
                              <For each={r().events}>
                                {(ev) => (
                                  <li><a class="row" href={importPath(ev.url)}>
                                    <span class="grow">
                                      <span class="name">{ev.title}</span><br />
                                      <span class="meta">{[ev.venue, ev.city, formatRange(ev.starts_on, ev.ends_on)].filter(Boolean).join(" · ")}</span>
                                    </span>
                                    <span class="chip new">importer</span>
                                  </a></li>
                                )}
                              </For>
                            </ul>
                          </Show>
                        </Show>
                      </section>
                    )}
                  </Show>
                </Loading>
              )}
            </For>
          </Show>
        </Show>

        <a class="btn block" href={`/spectacles/nouveau${query() ? `?titre=${encodeURIComponent(query())}` : ""}`}>＋ Saisir un spectacle à la main</a>
      </main>
    </>
  );
}

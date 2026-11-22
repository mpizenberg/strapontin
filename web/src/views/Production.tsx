import { createMemo, createSignal, For, refresh, Show } from "solid-js";
import { api } from "../api";
import { back, navigate, route } from "../router";
import { roleLabel, roles } from "../roles";
import { aria, capitalize, formatDay, formatLongDay, formatRange, plural } from "../format";
import { Poster } from "../components/Poster";
import { Stars } from "../components/Stars";
import { OutingSheet } from "../components/OutingSheet";
import { SummaryRow } from "../components/SummaryRow";
import type { Credit, Outing, Production } from "../types";

/** Credits grouped by role, in the order of the role list. */
function groupCredits(credits: Credit[]) {
  return roles
    .map((r) => ({ role: r.key, label: r.label, credits: credits.filter((c) => c.role === r.key) }))
    .filter((g) => g.credits.length > 0);
}

const headline = ["author", "adaptation", "director"];

export function ProductionView(props: { id: number }) {
  const p = createMemo(() => api.production(props.id));
  // null: closed; "new": a new viewing; an outing: edit it.
  const [sheet, setSheet] = createSignal<Outing | "new" | null>(null);
  const reload = () => {
    setSheet(null);
    refresh(p);
  };

  // A link from À voir ("Vu" on a finished run) opens the viewing form directly.
  // ?vu=nouveau follows "Déjà vu" on a new production.
  const vu = () => route().query.get("vu");
  const sheetOuting = (): Outing | "new" | null => {
    const s = sheet();
    if (s) return s;
    if (vu() === "nouveau") return "new";
    return vu() ? p().outings.find((o) => String(o.id) === vu()) ?? null : null;
  };
  const closeSheet = () => {
    setSheet(null);
    if (vu()) navigate(`/spectacles/${props.id}`, { replace: true });
  };

  const wish = async () => {
    await api.createOuting({ production_id: props.id, status: "want", run_id: null, date: "", rating: null, comment: "" });
    refresh(p);
  };
  const removeOuting = async (o: Outing) => {
    await api.deleteOuting(o.id);
    refresh(p);
  };
  const toggleFavorite = async (c: Credit) => {
    await api.setFavorite("person", c.person.id, !c.favorite);
    refresh(p);
  };

  return (
    <>
      <header class="appbar">
        <button class="appbar-back link" aria-label="Retour" onClick={() => back()}>‹</button>
        <h1>Spectacle</h1>
        <a class="btn ghost small" href={`/spectacles/${props.id}/modifier`}>Modifier</a>
      </header>
      <main class="page">
        <div class="media">
          <Poster id={p().id} title={p().title} src={p().poster} size="xl" />
          <div class="media-body">
            <h2 class="title">{p().title}</h2>
            <p class="meta">
              <For each={p().credits.filter((c) => headline.includes(c.role))}>
                {(c) => (
                  <>
                    <Show when={c.role !== "author"}>{capitalize(roleLabel(c.role))} </Show>
                    <a class="link" href={`/personnes/${c.person.id}`}>{c.person.name}</a>
                    <Show when={c.favorite}> <span class="star">★</span></Show>
                    <br />
                  </>
                )}
              </For>
              {p().company}
            </p>
          </div>
        </div>

        <section class="section">
          <h2 class="section-title">
            Mes sorties
            <Show when={p().outings.some((o) => o.status === "seen")}>
              <button class="link" onClick={() => setSheet("new")}>＋ Revu</button>
            </Show>
          </h2>
          <Show when={p().outings.length > 0} fallback={
            <div class="btn-row">
              <button class="btn primary" onClick={wish}>Envie</button>
              <button class="btn" onClick={() => setSheet("new")}>Déjà vu</button>
            </div>
          }>
            <ul class="list">
              <For each={p().outings}>
                {(o) => (
                  <li>
                    <Show when={o.status === "seen"} fallback={
                      <div class="row">
                        <span class="grow">
                          <span class="chip status-want">Envie</span>
                          <br />
                          <span class="meta">Dans « À voir »</span>
                        </span>
                        <button class="btn small" onClick={() => removeOuting(o)}>Retirer</button>
                        <button class="btn small primary" onClick={() => setSheet(o)}>Je l'ai vu</button>
                      </div>
                    }>
                      <button class="row" onClick={() => setSheet(o)}>
                        <span class="grow">
                          <span class="name">{o.date ? formatLongDay(o.date) : "Date inconnue"}</span>{" "}
                          <Show when={o.rating}>{(r) => <Stars rating={r()} />}</Show>
                          <br />
                          <span class="meta">{o.venue?.name}</span>
                          <Show when={o.comment}><span class="excerpt meta">{o.comment}</span></Show>
                        </span>
                        <span class="chev">›</span>
                      </button>
                    </Show>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </section>

        <Show when={p().runs.length > 0}>
          <section class="section">
            <h2 class="section-title">Représentations</h2>
            <ul class="list">
              <For each={p().runs}>
                {(r) => (
                  <li>
                    <a class="row" href={`/lieux/${r.venue.id}`}>
                      <span class="grow">
                        <span class="name">{r.venue.name}</span>
                        <br />
                        <span class="meta">{[r.venue.city, formatRange(r.starts_on, r.ends_on)].filter(Boolean).join(" · ")}</span>
                      </span>
                      <span class="chev">›</span>
                    </a>
                  </li>
                )}
              </For>
            </ul>
          </section>
        </Show>

        <Show when={p().credits.length > 0}>
          <section class="section">
            <h2 class="section-title">Distribution et équipe</h2>
            <div class="list">
              <For each={groupCredits(p().credits)}>
                {(g) => (
                  <div class="credit-group">
                    <h3>{capitalize(g.label)}</h3>
                    <For each={g.credits}>
                      {(c) => (
                        <div class="credit-row">
                          <a class="who" href={`/personnes/${c.person.id}`}>
                            <span class="name">{c.person.name}</span>
                            <Show when={c.character}><br /><span class="character">{c.character}</span></Show>
                          </a>
                          <Show when={c.seen > 0}><span class="meta">{plural(c.seen, "vu", "vus")}</span></Show>
                          <button class={["icon-btn", { star: c.favorite }]} aria-pressed={aria(c.favorite)}
                            aria-label={c.favorite ? "Retirer des favoris" : "Ajouter aux favoris"} onClick={() => toggleFavorite(c)}>
                            {c.favorite ? "★" : "☆"}
                          </button>
                        </div>
                      )}
                    </For>
                  </div>
                )}
              </For>
            </div>
          </section>
        </Show>

        <Show when={p().description}>
          <details class="card">
            <summary class="section-title">Présentation</summary>
            <p class="description">{p().description}</p>
          </details>
        </Show>

        <Show when={p().others.length > 0}>
          <section class="section">
            <h2 class="section-title">Autres mises en scène du texte</h2>
            <ul class="list">
              <For each={p().others}>{(s) => <li><SummaryRow summary={s} /></li>}</For>
            </ul>
          </section>
        </Show>

        <Footer p={p()} />
      </main>

      <Show when={sheetOuting()}>
        {(s) => (
          <OutingSheet production={p()} outing={s() === "new" ? undefined : (s() as Outing)} onClose={closeSheet} onSaved={() => { closeSheet(); reload(); }} />
        )}
      </Show>
    </>
  );
}

function Footer(props: { p: Production }) {
  const host = () => {
    try {
      return new URL(props.p.url!).host.replace(/^www\./, "");
    } catch {
      return "";
    }
  };
  return (
    <p class="meta">
      Fiche créée le {formatDay(props.p.created_at)}
      <Show when={props.p.revisions > 0}> · modifiée {plural(props.p.revisions, "fois", "fois")}</Show>
      <Show when={host()}>
        {" · "}
        <a class="link" href={props.p.url} target="_blank" rel="noopener noreferrer">{host()} ↗</a>
      </Show>
    </p>
  );
}

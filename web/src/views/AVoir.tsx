import { createMemo, For, refresh, Show } from "solid-js";
import { api } from "../api";
import { daysUntil, formatDay, formatRange, plural, today } from "../format";
import { SummaryRow } from "../components/SummaryRow";
import type { Outing } from "../types";

/** Wishes ordered by urgency: running and upcoming by end of run, undated, then finished runs. */
function sortWishes(list: Outing[]) {
  const now = today();
  const end = (o: Outing) => o.production?.ends_on ?? o.production?.starts_on;
  const dated = list.filter((o) => end(o) && end(o)! >= now).sort((a, b) => end(a)!.localeCompare(end(b)!));
  const undated = list.filter((o) => !end(o));
  const over = list.filter((o) => end(o) && end(o)! < now).sort((a, b) => end(b)!.localeCompare(end(a)!));
  return { dated, undated, over };
}

export function AVoir(props: { name: string }) {
  const wishes = createMemo(() => api.outings("want"));
  const groups = createMemo(() => sortWishes(wishes()));

  const remove = async (o: Outing) => {
    await api.deleteOuting(o.id);
    refresh(wishes);
  };

  return (
    <>
      <header class="appbar">
        <h1>À voir</h1>
        <a class="btn primary small" href="/chercher">＋ Ajouter</a>
      </header>
      <main class="page">
        <Show when={wishes().length > 0} fallback={<Welcome name={props.name} />}>
          <Show when={groups().dated.length + groups().undated.length > 0}>
            <section class="section">
              <h2 class="section-title">Envie</h2>
              <ul class="list">
                <For each={[...groups().dated, ...groups().undated]}>
                  {(o) => <li><WishRow outing={o} /></li>}
                </For>
              </ul>
            </section>
          </Show>
          <Show when={groups().over.length > 0}>
            <section class="section">
              <h2 class="section-title">C'est fini</h2>
              <ul class="list">
                <For each={groups().over}>
                  {(o) => (
                    <li>
                      <SummaryRow
                        summary={o.production!}
                        meta={[o.production!.venue?.name, `dernière le ${formatDay(o.production!.ends_on ?? o.production!.starts_on!)}`].filter(Boolean).join(" · ")}
                        aside={
                          <>
                            <button class="btn small" onClick={(e) => { e.preventDefault(); remove(o); }}>Retirer</button>
                            <a class="btn small" href={`/spectacles/${o.production_id}?vu=${o.id}`}>Vu</a>
                          </>
                        }
                      />
                    </li>
                  )}
                </For>
              </ul>
            </section>
          </Show>
        </Show>
      </main>
    </>
  );
}

function WishRow(props: { outing: Outing }) {
  const p = () => props.outing.production!;
  const left = () => (p().ends_on ? daysUntil(p().ends_on!) : undefined);
  const when = () => {
    const s = p();
    if (!s.starts_on && !s.ends_on) return "dates inconnues";
    if (s.starts_on && s.starts_on > today()) return formatRange(s.starts_on, s.ends_on);
    return `jusqu'au ${formatDay(s.ends_on ?? s.starts_on!)}`;
  };
  return (
    <SummaryRow
      summary={p()}
      meta={[p().venue?.name, when()].filter(Boolean).join(" · ")}
      aside={
        <Show when={left() !== undefined && left()! <= 14 && (!p().starts_on || p().starts_on! <= today())} fallback={<span class="chev">›</span>}>
          <span class="chip warn">{left() === 0 ? "dernier jour" : `plus que ${plural(left()!, "jour", "jours")}`}</span>
        </Show>
      }
    />
  );
}

function Welcome(props: { name: string }) {
  return (
    <>
      <div class="empty-state">
        <p class="title">Bienvenue, {props.name.split(" ")[0]}</p>
        <p>Notez ici les spectacles qui vous tentent. Ceux que vous avez vus iront dans le Journal.</p>
        <a class="btn primary" href="/chercher">＋ Ajouter un premier spectacle</a>
      </div>
      <section class="section">
        <h2 class="section-title">Le plus simple</h2>
        <ol class="list">
          <li class="row"><span class="grow">Ouvrez la page du spectacle sur le site du théâtre.</span></li>
          <li class="row"><span class="grow">Copiez son adresse, ou utilisez « Partager » → Strapontin sur Android.</span></li>
          <li class="row"><span class="grow">Collez-la dans « Chercher » et vérifiez la fiche.</span></li>
        </ol>
      </section>
    </>
  );
}

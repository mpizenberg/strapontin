import { createMemo, For, Show } from "solid-js";
import { api } from "../api";
import { formatDay, formatMonth, plural } from "../format";
import { Poster } from "../components/Poster";
import { Stars } from "../components/Stars";
import type { Outing } from "../types";

function byMonth(list: Outing[]) {
  const groups: { month: string; outings: Outing[] }[] = [];
  for (const o of list) {
    const month = (o.date ?? o.created_at).slice(0, 7);
    if (groups.at(-1)?.month !== month) groups.push({ month, outings: [] });
    groups.at(-1)!.outings.push(o);
  }
  return groups;
}

export function Journal() {
  const seen = createMemo(() => api.outings("seen"));
  return (
    <>
      <header class="appbar">
        <h1>Journal</h1>
        <span class="meta">{plural(seen().length, "sortie", "sorties")}</span>
      </header>
      <main class="page">
        <Show when={seen().length > 0} fallback={
          <div class="empty-state">
            <p class="title">Rien encore</p>
            <p>Les spectacles que vous marquez « Vu » s'affichent ici, du plus récent au plus ancien.</p>
          </div>
        }>
          <For each={byMonth(seen())}>
            {(g) => (
              <section class="section">
                <h2 class="section-title">{formatMonth(g.month + "-01")}</h2>
                <ul class="list">
                  <For each={g.outings}>
                    {(o) => (
                      <li>
                        <a class="row journal-entry" href={`/spectacles/${o.production_id}`}>
                          <Poster id={o.production_id} title={o.production!.title} src={o.production!.poster} />
                          <span class="grow">
                            <span class="name">{o.production!.title}</span>{" "}
                            <Show when={o.rating} fallback={<span class="meta">sans note</span>}>
                              {(r) => <Stars rating={r()} />}
                            </Show>
                            <br />
                            <span class="meta">
                              {[o.date && formatDay(o.date), o.venue?.name].filter(Boolean).join(" · ")}
                            </span>
                            <Show when={o.comment}>
                              <span class="excerpt">{o.comment}</span>
                            </Show>
                          </span>
                        </a>
                      </li>
                    )}
                  </For>
                </ul>
              </section>
            )}
          </For>
        </Show>
      </main>
    </>
  );
}

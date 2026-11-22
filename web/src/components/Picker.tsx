import { createMemo, createSignal, For, Loading, Show } from "solid-js";
import { api } from "../api";
import { Sheet } from "./Sheet";
import type { PersonRef, VenueRef, WorkRef } from "../types";

type Kinds = { person: PersonRef; venue: VenueRef; work: WorkRef };

const titles = { person: "Choisir une personne", venue: "Choisir un lieu", work: "Choisir le texte" };
const labels = { person: "Nom", venue: "Nom du lieu", work: "Titre du texte" };

/**
 * Finds an existing person, venue or work in the catalogue, or creates a new
 * one from the typed text. New references carry no id.
 */
export function Picker<K extends keyof Kinds>(props: { kind: K; initial?: string; onPick: (ref: Kinds[K]) => void; onClose: () => void }) {
  const [text, setText] = createSignal(() => props.initial ?? "");
  const [city, setCity] = createSignal("");
  const results = createMemo(() => (text().trim().length < 2 ? null : api.search(text(), props.kind)));
  const list = () => {
    const r = results();
    if (!r) return [];
    if (props.kind === "person") return r.people.map((p) => ({ ref: p, label: p.name, meta: "" }));
    if (props.kind === "venue") return r.venues.map((v) => ({ ref: v, label: v.name, meta: v.city ?? "" }));
    return r.works.map((w) => ({ ref: w, label: w.title, meta: "" }));
  };
  const pick = (ref: PersonRef | VenueRef | WorkRef) => props.onPick(ref as Kinds[K]);
  const create = () => {
    const name = text().trim().replace(/\s+/g, " ");
    if (!name) return;
    if (props.kind === "work") pick({ title: name });
    else if (props.kind === "venue") pick({ name, city: city().trim() });
    else pick({ name });
  };

  return (
    <Sheet title={titles[props.kind]} onClose={props.onClose}>
      <div class="field">
        <label for="picker-text">{labels[props.kind]}</label>
        <input class="input" id="picker-text" autocomplete="off" value={text()} onInput={(e) => setText(e.currentTarget.value)} />
      </div>
      <Loading fallback={<p class="loading">Recherche…</p>}>
        <Show when={list().length > 0}>
          <ul class="list">
            <For each={list()}>
              {(item) => (
                <li>
                  <button class="row" onClick={() => pick(item.ref)}>
                    <span class="grow">
                      <span class="name">{item.label}</span>
                      <Show when={item.meta}><br /><span class="meta">{item.meta}</span></Show>
                    </span>
                    <span class="chip ok">existante</span>
                  </button>
                </li>
              )}
            </For>
          </ul>
        </Show>
      </Loading>
      <Show when={text().trim()}>
        <Show when={props.kind === "venue"}>
          <div class="field">
            <label for="picker-city">Ville</label>
            <input class="input" id="picker-city" value={city()} onInput={(e) => setCity(e.currentTarget.value)} />
          </div>
        </Show>
        <button class="btn block" onClick={create}>Créer une fiche « {text().trim()} »</button>
      </Show>
    </Sheet>
  );
}

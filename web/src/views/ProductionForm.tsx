import { createSignal, createStore, For, Show } from "solid-js";
import { api } from "../api";
import { back, navigate, route } from "../router";
import { roles } from "../roles";
import { Picker } from "../components/Picker";
import type { PersonRef, Production, ProductionInput, Status, VenueRef } from "../types";

function fromProduction(p: Production): ProductionInput {
  return {
    title: p.title,
    work: p.work,
    company: p.company ?? "",
    description: p.description ?? "",
    url: p.url ?? "",
    credits: p.credits.map((c) => ({ person: c.person, role: c.role, character: c.character ?? "" })),
    runs: p.runs.map((r) => ({ id: r.id, venue: r.venue, starts_on: r.starts_on ?? "", ends_on: r.ends_on ?? "" })),
  };
}

function empty(title: string): ProductionInput {
  return { title, work: null, company: "", description: "", url: "", credits: [], runs: [] };
}

type Picking =
  | { kind: "work" }
  | { kind: "person"; index?: number }
  | { kind: "venue"; index?: number };

const Match = (props: { known: boolean }) => (
  <span class={["chip", props.known ? "ok" : "new"]}>{props.known ? "existante" : "nouvelle"}</span>
);

/** Creates a production by hand (no id) or edits one. */
export function ProductionForm(props: { id?: number }) {
  const [form, setForm] = createStore(async () =>
    props.id ? fromProduction(await api.production(props.id)) : empty(route().query.get("titre") ?? ""),
  );
  const [picking, setPicking] = createSignal<Picking | null>(null);
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);

  const onPick = (ref: PersonRef | VenueRef | { title: string; id?: number }) => {
    const p = picking()!;
    setForm((f) => {
      if (p.kind === "work") f.work = ref as { title: string };
      else if (p.kind === "person") {
        if (p.index === undefined) f.credits.push({ person: ref as PersonRef, role: "performer", character: "" });
        else f.credits[p.index].person = ref as PersonRef;
      } else {
        if (p.index === undefined) f.runs.push({ venue: ref as VenueRef, starts_on: "", ends_on: "" });
        else f.runs[p.index].venue = ref as VenueRef;
      }
    });
    setPicking(null);
  };

  const save = async (status?: Status) => {
    setBusy(true);
    setError("");
    try {
      const input = JSON.parse(JSON.stringify(form)) as ProductionInput;
      if (props.id) {
        await api.updateProduction(props.id, input);
        navigate(`/spectacles/${props.id}`, { replace: true });
      } else {
        const { id } = await api.createProduction(status === "want" ? { ...input, status } : input);
        navigate(`/spectacles/${id}${status === "seen" ? "?vu=nouveau" : ""}`, { replace: true });
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <header class="appbar">
        <button class="appbar-back link" aria-label="Retour" onClick={() => back()}>‹</button>
        <h1>{props.id ? "Modifier la fiche" : "Nouveau spectacle"}</h1>
      </header>
      <main class="page has-footer">
        <div class="field">
          <label for="f-title">Titre</label>
          <input class="input" id="f-title" required value={form.title} onInput={(e) => setForm((f) => { f.title = e.currentTarget.value; })} />
        </div>

        <div class="field">
          <span class="label">Texte</span>
          <Show when={form.work} fallback={
            <button class="picked empty" onClick={() => setPicking({ kind: "work" })}>＋ Lier au texte (Molière, Le Misanthrope…)</button>
          }>
            {(w) => (
              <div class="input-group">
                <button class="picked grow" onClick={() => setPicking({ kind: "work" })}>
                  <span class="grow">{w().title}</span>
                  <Match known={!!w().id} />
                </button>
                <button class="icon-btn" aria-label="Retirer le texte" onClick={() => setForm((f) => { f.work = null; })}>×</button>
              </div>
            )}
          </Show>
        </div>

        <div class="field">
          <label for="f-company">Compagnie</label>
          <input class="input" id="f-company" value={form.company} onInput={(e) => setForm((f) => { f.company = e.currentTarget.value; })} />
        </div>

        <section class="section">
          <h2 class="section-title">Représentations</h2>
          <div class="list">
            <For each={form.runs}>
              {(r, i) => (
                <div class="credit-row run-row">
                  <div class="who stack">
                    <button class="picked" onClick={() => setPicking({ kind: "venue", index: i() })}>
                      <span class="grow">{r.venue.name}<Show when={r.venue.city}><span class="meta"> · {r.venue.city}</span></Show></span>
                      <Match known={!!r.venue.id} />
                    </button>
                    <div class="two-cols">
                      <label class="field"><span class="label">Du</span>
                        <input class="input" type="date" value={r.starts_on} onInput={(e) => setForm((f) => { f.runs[i()].starts_on = e.currentTarget.value; })} />
                      </label>
                      <label class="field"><span class="label">Au</span>
                        <input class="input" type="date" value={r.ends_on} onInput={(e) => setForm((f) => { f.runs[i()].ends_on = e.currentTarget.value; })} />
                      </label>
                    </div>
                  </div>
                  <button class="icon-btn" aria-label="Retirer" onClick={() => setForm((f) => { f.runs.splice(i(), 1); })}>×</button>
                </div>
              )}
            </For>
            <button class="add-line" onClick={() => setPicking({ kind: "venue" })}>＋ Ajouter un lieu</button>
          </div>
        </section>

        <section class="section">
          <h2 class="section-title">Crédits ({form.credits.length})</h2>
          <div class="list">
            <For each={form.credits}>
              {(c, i) => (
                <div class="credit-row">
                  <div class="who stack">
                    <span>
                      <button class="link name" onClick={() => setPicking({ kind: "person", index: i() })}>{c.person.name}</button>{" "}
                      <Match known={!!c.person.id} />
                    </span>
                    <div class="input-group">
                      <select class="select" aria-label="Rôle" onChange={(e) => setForm((f) => { f.credits[i()].role = e.currentTarget.value; })}>
                        <For each={roles}>{(r) => <option value={r.key} selected={r.key === c.role}>{r.label}</option>}</For>
                      </select>
                      <Show when={c.role === "performer"}>
                        <input class="input" placeholder="Personnage" aria-label="Personnage" value={c.character}
                          onInput={(e) => setForm((f) => { f.credits[i()].character = e.currentTarget.value; })} />
                      </Show>
                    </div>
                  </div>
                  <button class="icon-btn" aria-label="Retirer" onClick={() => setForm((f) => { f.credits.splice(i(), 1); })}>×</button>
                </div>
              )}
            </For>
            <button class="add-line" onClick={() => setPicking({ kind: "person" })}>＋ Ajouter une personne</button>
          </div>
        </section>

        <div class="field">
          <label for="f-description">Présentation</label>
          <textarea class="textarea" id="f-description" rows="5" value={form.description}
            onInput={(e) => setForm((f) => { f.description = e.currentTarget.value; })} />
        </div>

        <div class="field">
          <label for="f-url">Page du spectacle</label>
          <input class="input" id="f-url" type="url" placeholder="https://" value={form.url}
            onInput={(e) => setForm((f) => { f.url = e.currentTarget.value; })} />
        </div>

        <Show when={error()}>
          <p class="notice warn" role="alert">{error()}</p>
        </Show>
      </main>

      <div class="footer-actions">
        <Show when={props.id} fallback={
          <>
            <span class="label">Enregistrer dans mon Strapontin :</span>
            <div class="btn-row">
              <button class="btn primary" disabled={busy()} onClick={() => save("want")}>Envie</button>
              <button class="btn" disabled={busy()} onClick={() => save("seen")}>Déjà vu</button>
            </div>
          </>
        }>
          <button class="btn primary block" disabled={busy()} onClick={() => save()}>Enregistrer</button>
        </Show>
      </div>

      <Show when={picking()}>
        {(p) => (
          <Picker
            kind={p().kind}
            initial={p().kind === "person" && (p() as { index?: number }).index !== undefined ? form.credits[(p() as { index: number }).index].person.name : ""}
            onPick={onPick}
            onClose={() => setPicking(null)}
          />
        )}
      </Show>
    </>
  );
}

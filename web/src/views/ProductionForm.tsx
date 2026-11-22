import { createMemo, createSignal, createStore, Errored, For, Loading, Show } from "solid-js";
import { api } from "../api";
import { back, navigate } from "../router";
import { roles } from "../roles";
import { Picker } from "../components/Picker";
import { SummaryRow } from "../components/SummaryRow";
import type { ImportDraft, PersonRef, Production, ProductionInput, Status, Summary, VenueRef, WorkRef } from "../types";

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

type Source = { input?: ProductionInput; draft?: ImportDraft; duplicate?: Summary };

/**
 * One form for manual entry, import review and edits. With id it edits; with
 * importURL it reads the page first (the server reuses a recent snapshot, so
 * reloading is cheap); otherwise it starts blank.
 */
export function ProductionForm(props: { id?: number; importURL?: string; title?: string }) {
  const source = createMemo(async (): Promise<Source> => {
    if (props.id) return { input: fromProduction(await api.production(props.id)) };
    if (props.importURL) {
      const r = await api.importURL(props.importURL);
      return r.duplicate ? { duplicate: r.duplicate } : { input: r.draft!, draft: r.draft };
    }
    return { input: empty(props.title ?? "") };
  });

  return (
    <>
      <header class="appbar">
        <button class="appbar-back link" aria-label="Retour" onClick={() => back("/chercher")}>‹</button>
        <h1>{props.id ? "Modifier la fiche" : props.importURL ? "Vérifier la fiche" : "Nouveau spectacle"}</h1>
      </header>
      <Errored fallback={(err) => <ImportFailed message={err() instanceof Error ? (err() as Error).message : String(err())} />}>
        <Loading fallback={<main class="page"><div class="card"><p class="loading">{props.importURL ? "Lecture de la page…" : "Chargement…"}</p></div></main>}>
          <Show when={source().duplicate} fallback={<Form id={props.id} initial={source().input!} draft={source().draft} />}>
            {(dup) => <Duplicate summary={dup()} />}
          </Show>
        </Loading>
      </Errored>
    </>
  );
}

function ImportFailed(props: { message: string }) {
  return (
    <main class="page">
      <div class="notice warn" role="alert">
        <div><strong>La page n'a pas pu être lue</strong>{props.message}</div>
      </div>
      <a class="btn block" href="/spectacles/nouveau">Saisir le spectacle à la main</a>
    </main>
  );
}

function Duplicate(props: { summary: Summary }) {
  const wish = async () => {
    await api.createOuting({ production_id: props.summary.id, status: "want", run_id: null, date: "", rating: null, comment: "" });
    navigate(`/spectacles/${props.summary.id}`, { replace: true });
  };
  return (
    <main class="page">
      <div class="notice ok">
        <div>
          <strong>Déjà dans Strapontin</strong>
          {props.summary.mine ? "Ce spectacle est déjà dans votre Strapontin." : "Un membre a déjà enregistré cette page."}
        </div>
      </div>
      <ul class="list"><li><SummaryRow summary={props.summary} /></li></ul>
      <Show when={!props.summary.mine}>
        <section class="section">
          <h2 class="section-title">Ajouter à mon Strapontin</h2>
          <div class="btn-row">
            <button class="btn primary" onClick={wish}>Envie</button>
            <a class="btn" href={`/spectacles/${props.summary.id}?vu=nouveau`}>Déjà vu</a>
          </div>
        </section>
      </Show>
    </main>
  );
}

type Picking = { kind: "work" } | { kind: "person"; index?: number } | { kind: "venue"; index?: number };

const Known = (props: { known: boolean }) => (
  <span class={["chip", props.known ? "ok" : "new"]}>{props.known ? "existante" : "nouvelle"}</span>
);

const host = (url: string) => {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
};

function Form(props: { id?: number; initial: ProductionInput; draft?: ImportDraft }) {
  const [form, setForm] = createStore(() => structuredClone(props.initial) as ProductionInput);
  const [picking, setPicking] = createSignal<Picking | null>(null);
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const poor = () => props.draft && (props.draft.source === "opengraph" || props.draft.source === "html");

  const onPick = (ref: PersonRef | VenueRef | WorkRef) => {
    const p = picking()!;
    setForm((f) => {
      if (p.kind === "work") f.work = ref as WorkRef;
      else if (p.kind === "person") {
        if (p.index === undefined) f.credits.push({ person: ref as PersonRef, role: "performer", character: "" });
        else f.credits[p.index] = { ...f.credits[p.index], person: ref as PersonRef, candidates: undefined };
      } else {
        if (p.index === undefined) f.runs.push({ venue: ref as VenueRef, starts_on: "", ends_on: "" });
        else f.runs[p.index] = { ...f.runs[p.index], venue: ref as VenueRef, guessed: false };
      }
    });
    setPicking(null);
  };

  const pickerInitial = () => {
    const p = picking();
    if (p?.kind === "person" && p.index !== undefined) return form.credits[p.index].person.name;
    if (p?.kind === "venue" && p.index !== undefined) return form.runs[p.index].venue.name;
    if (p?.kind === "work") return form.work?.title ?? form.title;
    return "";
  };

  const save = async (status?: Status) => {
    setBusy(true);
    setError("");
    try {
      const input = JSON.parse(JSON.stringify(form)) as ProductionInput;
      if (props.id) {
        await api.updateProduction(props.id, input);
        navigate(`/spectacles/${props.id}`, { replace: true });
        return;
      }
      const { id } = await api.createProduction({
        ...input,
        status: status === "want" ? status : undefined,
        snapshot_id: props.draft?.snapshot_id,
        image: props.draft?.image,
      });
      navigate(`/spectacles/${id}${status === "seen" ? "?vu=nouveau" : ""}`, { replace: true });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <main class="page has-footer">
        <Show when={props.draft}>
          {(d) => (
            <div class="stack">
              <p class="meta">
                Lu sur <b>{host(d().url)}</b>. Vérifiez et complétez avant d'enregistrer.
                {/* The poster isn't previewed: the page only loads images from Strapontin itself. */}
                <Show when={d().image}> L'affiche sera enregistrée avec la fiche.</Show>
              </p>
              <Show when={poor()}>
                <p class="notice warn">Cette page ne donne presque rien de structuré : ajoutez le lieu, les dates et les crédits.</p>
              </Show>
            </div>
          )}
        </Show>

        <div class="field">
          <label for="f-title">Titre</label>
          <input class="input" id="f-title" required value={form.title} onInput={(e) => setForm((f) => { f.title = e.currentTarget.value; })} />
        </div>

        <div class="field">
          <span class="label">Texte</span>
          <Show when={form.work} fallback={
            <button class="picked empty" onClick={() => setPicking({ kind: "work" })}>＋ Lier au texte (Le Misanthrope, Phèdre…)</button>
          }>
            {(w) => (
              <div class="input-group">
                <button class="picked grow" onClick={() => setPicking({ kind: "work" })}>
                  <span class="grow">{w().title}</span>
                  <Known known={!!w().id} />
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
                    <Show when={r.venue.name} fallback={
                      <button class="picked empty" onClick={() => setPicking({ kind: "venue", index: i() })}>＋ Choisir le lieu</button>
                    }>
                      <button class="picked" onClick={() => setPicking({ kind: "venue", index: i() })}>
                        <span class="grow">{r.venue.name}<Show when={r.venue.city}><span class="meta"> · {r.venue.city}</span></Show></span>
                        <Show when={r.guessed} fallback={<Known known={!!r.venue.id} />}>
                          <span class="chip warn">à confirmer</span>
                        </Show>
                      </button>
                    </Show>
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
                <div class={["credit-row", { attention: (c.candidates?.length ?? 0) > 1 }]}>
                  <div class="who stack">
                    <span>
                      <button class="link name" onClick={() => setPicking({ kind: "person", index: i() })}>{c.person.name}</button>{" "}
                      <Show when={(c.candidates?.length ?? 0) > 1} fallback={<Known known={!!c.person.id} />}>
                        <span class="chip warn">{c.candidates!.length} homonymes : choisir</span>
                      </Show>
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
        {(p) => <Picker kind={p().kind} initial={pickerInitial()} onPick={onPick} onClose={() => setPicking(null)} />}
      </Show>
    </>
  );
}

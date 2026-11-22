import { createSignal, For, Show } from "solid-js";
import { api } from "../api";
import { formatRange, today } from "../format";
import { Sheet } from "./Sheet";
import { StarsInput } from "./Stars";
import type { Outing, Production } from "../types";

/**
 * The form for a viewing: date, run, rating, private comment. With an outing
 * it edits it (a wish becomes a viewing); without, it adds a new viewing.
 */
export function OutingSheet(props: { production: Production; outing?: Outing; onClose: () => void; onSaved: () => void }) {
  const o = () => props.outing;
  const defaultRun = () => o()?.run_id ?? (props.production.runs.length === 1 ? props.production.runs[0].id : null);
  const [date, setDate] = createSignal(() => o()?.date ?? today());
  const [runID, setRunID] = createSignal<number | null>(() => defaultRun());
  const [rating, setRating] = createSignal<number | null>(() => o()?.rating ?? null);
  const [comment, setComment] = createSignal(() => o()?.comment ?? "");
  const [error, setError] = createSignal("");

  const save = async () => {
    const input = { run_id: runID(), status: "seen" as const, date: date(), rating: rating(), comment: comment() };
    try {
      if (o()) await api.updateOuting(o()!.id, input);
      else await api.createOuting({ ...input, production_id: props.production.id });
      props.onSaved();
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const remove = async () => {
    await api.deleteOuting(o()!.id);
    props.onSaved();
  };

  return (
    <Sheet title={o()?.status === "seen" ? "Ma sortie" : "Je l'ai vu"} onClose={props.onClose}>
      <div class="two-cols">
        <div class="field">
          <label for="o-date">Date</label>
          <input class="input" id="o-date" type="date" value={date()} onInput={(e) => setDate(e.currentTarget.value)} />
        </div>
        <div class="field">
          <label for="o-run">Lieu</label>
          <select class="select" id="o-run" onChange={(e) => setRunID(e.currentTarget.value ? Number(e.currentTarget.value) : null)}>
            <option value="" selected={runID() === null}>—</option>
            <For each={props.production.runs}>
              {(r) => (
                <option value={r.id} selected={runID() === r.id}>
                  {r.venue.name}{r.starts_on || r.ends_on ? ` (${formatRange(r.starts_on, r.ends_on)})` : ""}
                </option>
              )}
            </For>
          </select>
        </div>
      </div>
      <div class="field">
        <span class="label">Ma note</span>
        <StarsInput value={rating()} onChange={setRating} />
      </div>
      <div class="field">
        <label for="o-comment">Mon commentaire</label>
        <textarea class="textarea" id="o-comment" rows="5" value={comment()} onInput={(e) => setComment(e.currentTarget.value)} />
        <span class="meta">Visible par vous uniquement.</span>
      </div>
      <Show when={error()}>
        <p class="notice warn" role="alert">{error()}</p>
      </Show>
      <div class="btn-row">
        <Show when={o()?.status === "seen"}>
          <button class="btn" onClick={remove}>Supprimer</button>
        </Show>
        <button class="btn primary" onClick={save}>Enregistrer</button>
      </div>
    </Sheet>
  );
}

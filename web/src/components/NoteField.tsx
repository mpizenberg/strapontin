import { createSignal } from "solid-js";
import { api } from "../api";

/** The member's private note on a person, venue or production, saved when the field loses focus. */
export function NoteField(props: { entity: string; id: number; body: string }) {
  const [body, setBody] = createSignal(() => props.body);
  const [saved, setSaved] = createSignal(false);
  const save = async () => {
    if (body() === props.body) return;
    await api.setNote(props.entity, props.id, body());
    setSaved(true);
  };
  return (
    <div class="field">
      <textarea class="textarea" rows="3" aria-label="Ma note" placeholder="Une remarque pour vous-même…"
        value={body()} onInput={(e) => { setBody(e.currentTarget.value); setSaved(false); }} onChange={save} />
      <span class="meta">{saved() ? "Enregistrée. " : ""}Visible par vous uniquement.</span>
    </div>
  );
}

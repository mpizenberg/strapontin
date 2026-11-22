import { For } from "solid-js";
import { aria } from "../format";

/** Read-only rating, five stars with the unearned ones dimmed. */
export function Stars(props: { rating: number }) {
  return (
    <span class="stars" aria-label={`${props.rating} sur 5`}>
      {"★".repeat(props.rating)}
      <span class="off">{"★".repeat(5 - props.rating)}</span>
    </span>
  );
}

/** Rating input: tapping the current rating clears it. */
export function StarsInput(props: { value: number | null; onChange: (v: number | null) => void }) {
  return (
    <span class="stars large" role="radiogroup" aria-label="Note">
      <For each={[1, 2, 3, 4, 5]}>
        {(n) => (
          <button
            type="button"
            role="radio"
            aria-checked={aria(props.value === n)}
            aria-label={`${n} sur 5`}
            class={{ off: (props.value ?? 0) < n }}
            onClick={() => props.onChange(props.value === n ? null : n)}
          >
            ★
          </button>
        )}
      </For>
    </span>
  );
}

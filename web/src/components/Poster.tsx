import { Show } from "solid-js";
import { posterClass } from "../format";

export function Poster(props: { id: number; title: string; src?: string; size?: "lg" | "xl" }) {
  return (
    <span class={["poster", posterClass(props.id), props.size]}>
      <Show when={props.src} fallback={props.title}>
        {(src) => <img src={src()} alt="" loading="lazy" />}
      </Show>
    </span>
  );
}

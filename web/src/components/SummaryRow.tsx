import { Show } from "solid-js";
import type { JSX } from "@solidjs/web";
import { Poster } from "./Poster";
import { year } from "../format";
import type { Summary } from "../types";

/** Chip with the member's status on a production. */
export function MineChip(props: { summary: Summary }) {
  return (
    <Show when={props.summary.mine}>
      {(m) => (
        <span class={["chip", `status-${m().status}`]}>
          {m().status === "seen" ? `Vu ${"★".repeat(m().rating ?? 0)}` : "Envie"}
        </span>
      )}
    </Show>
  );
}

/** A production in a list: poster, title, a meta line (venue and year by default), the member's status. */
export function SummaryRow(props: { summary: Summary; meta?: string; aside?: JSX.Element }) {
  const meta = () =>
    props.meta ?? [props.summary.venue?.name, year(props.summary.ends_on ?? props.summary.starts_on)].filter(Boolean).join(" · ");
  return (
    <a class="row" href={`/spectacles/${props.summary.id}`}>
      <Poster id={props.summary.id} title={props.summary.title} src={props.summary.poster} />
      <span class="grow">
        <span class="name">{props.summary.title}</span>
        <br />
        <span class="meta">{meta()}</span>
      </span>
      {props.aside ?? <MineChip summary={props.summary} />}
    </a>
  );
}

import { createMemo, createSignal, For, Show } from "solid-js";
import { api } from "../api";
import { capitalize, formatMonth, plural } from "../format";
import { roleLabel } from "../roles";
import { ShareLink } from "../components/ShareLink";
import type { InviteLink, Me } from "../types";

export function Moi(props: { onSignedOut: () => void }) {
  // Fetched again here: the counts change as the member uses the app.
  const me = createMemo(() => api.me() as Promise<Me>);
  const favorites = createMemo(() => api.favorites());
  const [link, setLink] = createSignal<InviteLink | null>(null);
  const invite = async () => setLink(await api.createInvite());
  const logout = async () => {
    await api.logout();
    props.onSignedOut();
  };

  return (
    <>
      <header class="appbar"><h1>Moi</h1></header>
      <main class="page">
        <div class="section">
          <h2 class="title">{me().name}</h2>
          <p class="meta">
            Membre depuis {formatMonth(me().created_at).toLowerCase()} · {plural(me().seen, "spectacle vu", "spectacles vus")} · {me().wanted} à voir
          </p>
        </div>

        <section class="section">
          <h2 class="section-title">Mes favoris</h2>
          <Show when={favorites().length > 0} fallback={
            <p class="meta">Touchez ☆ à côté d'un nom sur une fiche pour le retrouver ici.</p>
          }>
            <ul class="list">
              <For each={favorites()}>
                {(f) => (
                  <li>
                    <a class="row" href={f.entity === "person" ? `/personnes/${f.id}` : f.entity === "venue" ? `/lieux/${f.id}` : `/chercher?q=${encodeURIComponent(f.name)}`}>
                      <span class="star">★</span>
                      <span class="grow">
                        <span class="name">{f.name}</span>
                        <br />
                        <span class="meta">
                          {[
                            f.entity === "venue" ? ["Lieu", f.city].filter(Boolean).join(" · ") : capitalize(roleLabel(f.roles?.[0] ?? "")),
                            plural(f.productions, "spectacle", "spectacles"),
                          ].filter(Boolean).join(" · ")}
                        </span>
                      </span>
                      <span class="chev">›</span>
                    </a>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </section>

        <Show when={me().invites_left !== 0}>
          <section class="section">
            <h2 class="section-title">Inviter quelqu'un</h2>
            <div class="card">
              <Show when={link()} fallback={
                <>
                  <p>
                    {me().invites_left < 0 ? "Vous pouvez inviter qui vous voulez." : <>Il vous reste <b>{plural(me().invites_left, "invitation", "invitations")}</b>.</>}{" "}
                    Le lien est valable 7 jours et ne sert qu'une fois : envoyez-le par message.
                  </p>
                  <button class="btn primary" onClick={invite}>Créer un lien d'invitation</button>
                </>
              }>
                {(l) => <ShareLink link={l()} text="Je t'invite sur Strapontin, mon carnet de théâtre." />}
              </Show>
            </div>
          </section>
        </Show>

        <ul class="list">
          <Show when={me().role === "admin"}>
            <li><a class="row" href="/admin"><span class="grow">Administration</span><span class="chip">admin</span><span class="chev">›</span></a></li>
          </Show>
          <li><button class="row" onClick={logout}><span class="grow link">Se déconnecter</span></button></li>
        </ul>
      </main>
    </>
  );
}

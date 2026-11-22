import { createMemo, createSignal, For, refresh, Show } from "solid-js";
import { api } from "../api";
import { back } from "../router";
import { daysUntil, formatBytes, formatDay, formatMonth, plural } from "../format";
import { Sheet } from "../components/Sheet";
import { ShareLink } from "../components/ShareLink";
import type { AdminMember, InviteLink } from "../types";

/** Counts about the circle and the server. No comments, ratings or notes. */
export function Admin() {
  const o = createMemo(() => api.adminOverview());
  const members = createMemo(() => api.adminMembers());
  const [link, setLink] = createSignal<{ title: string; link: InviteLink } | null>(null);

  const backupAge = () => (o().last_backup ? -daysUntil(o().last_backup!) : undefined);
  const toggle = async (m: AdminMember) => {
    await api.adminMember(m.id, m.disabled ? "enable" : "disable");
    refresh(members);
  };

  return (
    <>
      <header class="appbar">
        <button class="appbar-back link" aria-label="Retour" onClick={() => back("/moi")}>‹</button>
        <h1>Administration</h1>
      </header>
      <main class="page">
        <section class="section">
          <h2 class="section-title">Cercle</h2>
          <div class="card">
            <p class="title">{o().members} / {o().settings.max_members} membres</p>
            <p class="meta">
              {plural(o().active_7, "actif", "actifs")} sur 7 jours · {o().active_30} sur 30 jours · profondeur d'invitation {o().max_depth}
            </p>
            <Show when={o().max_depth >= 3}>
              <p class="notice warn">Les invitations ont trois niveaux ou plus : le cercle s'élargit au-delà des proches.</p>
            </Show>
            <p class="meta">
              Inscriptions : <For each={o().signups}>{(c, i) => <>{i() > 0 ? ", " : ""}{formatMonth(c.key + "-01").toLowerCase()} ({c.count})</>}</For>
            </p>
          </div>
          <button class="btn" onClick={async () => setLink({ title: "Invitation", link: await api.createInvite() })}>Créer un lien d'invitation</button>
        </section>

        <section class="section">
          <h2 class="section-title">Membres</h2>
          <ul class="list">
            <For each={members()}>
              {(m) => (
                <li class="row">
                  <span class="grow">
                    <span class="name">{m.name}</span> <span class="meta">({m.login})</span>
                    <Show when={m.role === "admin"}> <span class="chip">admin</span></Show>
                    <Show when={m.disabled}> <span class="chip warn">compte suspendu</span></Show>
                    <br />
                    <span class="meta">
                      {[
                        m.invited_by ? `invitation : ${m.invited_by}` : "",
                        `inscription le ${formatDay(m.created_at)}`,
                        m.last_seen ? `dernière visite le ${formatDay(m.last_seen)}` : "aucune visite",
                        `${plural(m.outings, "sortie", "sorties")}, ${plural(m.imports, "import", "imports")}`,
                      ].filter(Boolean).join(" · ")}
                    </span>
                  </span>
                  <Show when={m.role !== "admin"}>
                    <button class="btn small" onClick={() => toggle(m)}>{m.disabled ? "Réactiver" : "Suspendre"}</button>
                  </Show>
                  <button class="btn small" onClick={async () => setLink({ title: `Mot de passe de ${m.name}`, link: await api.adminReset(m.id) })}>
                    Mot de passe
                  </button>
                </li>
              )}
            </For>
          </ul>
        </section>

        <section class="section">
          <h2 class="section-title">Activité (14 jours)</h2>
          <div class="card">
            <p class="meta">
              Imports : {o().imports.reduce((n, c) => n + c.count, 0)} · pages lues : {o().fetches.reduce((n, c) => n + c.count, 0)} ·
              échecs de connexion (7 jours) : {o().login_failed}
            </p>
          </div>
        </section>

        <section class="section">
          <h2 class="section-title">Santé</h2>
          <div class="card">
            <p class="meta">
              Base : {formatBytes(o().db_bytes)} · {plural(o().snapshots, "page conservée", "pages conservées")} ({formatBytes(o().snapshot_bytes)}) ·{" "}
              {plural(o().images, "affiche", "affiches")} ({formatBytes(o().image_bytes)})
            </p>
            <Show when={backupAge() !== undefined && backupAge()! < 2} fallback={
              <p class="notice warn">
                {o().last_backup ? `Dernière sauvegarde le ${formatDay(o().last_backup!)} : la sauvegarde quotidienne ne tourne plus.` : "Aucune sauvegarde pour l'instant."}
              </p>
            }>
              <p class="meta">Dernière sauvegarde le {formatDay(o().last_backup!)}.</p>
            </Show>
          </div>
        </section>

        <SettingsForm settings={o().settings} onSaved={() => refresh(o)} />
      </main>

      <Show when={link()}>
        {(l) => (
          <Sheet title={l().title} onClose={() => setLink(null)}>
            <ShareLink link={l().link} text="Ton lien pour Strapontin." />
          </Sheet>
        )}
      </Show>
    </>
  );
}

function SettingsForm(props: { settings: { max_members: number; invite_quota: number; ntfy_topic: string }; onSaved: () => void }) {
  const [max, setMax] = createSignal(() => String(props.settings.max_members));
  const [quota, setQuota] = createSignal(() => String(props.settings.invite_quota));
  const [topic, setTopic] = createSignal(() => props.settings.ntfy_topic);
  const [message, setMessage] = createSignal("");
  const save = async (e: SubmitEvent) => {
    e.preventDefault();
    try {
      await api.adminSettings({ max_members: Number(max()), invite_quota: Number(quota()), ntfy_topic: topic() });
      setMessage("Enregistré.");
      props.onSaved();
    } catch (err) {
      setMessage((err as Error).message);
    }
  };
  return (
    <form class="section" onSubmit={save}>
      <h2 class="section-title">Réglages</h2>
      <div class="two-cols">
        <label class="field"><span class="label">Membres au plus</span>
          <input class="input" type="number" min="1" value={max()} onInput={(e) => setMax(e.currentTarget.value)} /></label>
        <label class="field"><span class="label">Invitations par membre</span>
          <input class="input" type="number" min="0" value={quota()} onInput={(e) => setQuota(e.currentTarget.value)} /></label>
      </div>
      <label class="field"><span class="label">Sujet ntfy (vide : pas de notification)</span>
        <input class="input" value={topic()} onInput={(e) => setTopic(e.currentTarget.value)} /></label>
      <p class="meta">Une notification sans nom est envoyée à ntfy.sh quand quelqu'un rejoint Strapontin.</p>
      <Show when={message()}><p class="meta" role="status">{message()}</p></Show>
      <button class="btn primary block" type="submit">Enregistrer les réglages</button>
    </form>
  );
}

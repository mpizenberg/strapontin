import { createMemo, createSignal, Errored, Loading, Show } from "solid-js";
import { api } from "../api";
import { navigate } from "../router";

/** Redeems a signup or password reset link. Shown signed in or not. */
export function Invite(props: { token: string; onSignedIn: () => void }) {
  const invite = createMemo(() => api.checkInvite(props.token));
  const [name, setName] = createSignal("");
  const [login, setLogin] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);

  const submit = async (e: SubmitEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const body = invite().kind === "signup" ? { name: name(), login: login(), password: password() } : { password: password() };
      await api.redeemInvite(props.token, body);
      navigate("/", { replace: true });
      props.onSignedIn();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="center-page">
      <h1 class="brand">Strapontin</h1>
      <Errored fallback={(err) => (
        <>
          <p class="notice warn" role="alert">{err() instanceof Error ? (err() as Error).message : String(err())}</p>
          <a class="btn block" href="/">Aller à la connexion</a>
        </>
      )}>
        <Loading fallback={<p class="loading">Vérification du lien…</p>}>
          <form class="stack" onSubmit={submit}>
            <Show when={invite().kind === "signup"} fallback={
              <p>Choisissez un nouveau mot de passe pour <b>{invite().login}</b>.</p>
            }>
              <p><b>{invite().invited_by}</b> vous invite à rejoindre Strapontin, un carnet de théâtre partagé entre proches.</p>
              <div class="field">
                <label for="i-name">Votre nom</label>
                <input class="input" id="i-name" autocomplete="name" required value={name()} onInput={(e) => setName(e.currentTarget.value)} />
                <span class="meta">Visible par les autres membres.</span>
              </div>
              <div class="field">
                <label for="i-login">Identifiant</label>
                <input class="input" id="i-login" autocomplete="username" autocapitalize="none" required value={login()} onInput={(e) => setLogin(e.currentTarget.value)} />
              </div>
            </Show>
            <div class="field">
              <label for="i-password">Mot de passe</label>
              <input class="input" id="i-password" type="password" autocomplete="new-password" minlength="8" required
                value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
              <span class="meta">8 caractères au moins.</span>
            </div>
            <Show when={error()}>
              <p class="notice warn" role="alert">{error()}</p>
            </Show>
            <button class="btn primary block" type="submit" disabled={busy()}>
              {invite().kind === "signup" ? "Créer mon compte" : "Enregistrer le mot de passe"}
            </button>
          </form>
        </Loading>
      </Errored>
    </div>
  );
}

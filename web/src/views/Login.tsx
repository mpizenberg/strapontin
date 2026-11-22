import { createSignal, Show } from "solid-js";
import { api } from "../api";

export function Login(props: { onSignedIn: () => void }) {
  const [login, setLogin] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);

  const submit = async (e: SubmitEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.login(login(), password());
      props.onSignedIn();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form class="center-page" onSubmit={submit}>
      <h1 class="brand">Strapontin</h1>
      <div class="field">
        <label for="login">Identifiant</label>
        <input class="input" id="login" autocomplete="username" autocapitalize="none" required
          value={login()} onInput={(e) => setLogin(e.currentTarget.value)} />
      </div>
      <div class="field">
        <label for="password">Mot de passe</label>
        <input class="input" id="password" type="password" autocomplete="current-password" required
          value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
      </div>
      <Show when={error()}>
        <p class="notice warn" role="alert">{error()}</p>
      </Show>
      <button class="btn primary block" type="submit" disabled={busy()}>Se connecter</button>
      <p class="meta">Pas encore de compte ? Strapontin fonctionne sur invitation : demandez un lien à un membre.</p>
    </form>
  );
}

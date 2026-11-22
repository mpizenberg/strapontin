import { createMemo, Errored, Loading, Match, refresh, Switch } from "solid-js";
import { api, setUnauthorizedHandler } from "./api";
import { match, route } from "./router";
import { TabBar } from "./components/TabBar";
import { Login } from "./views/Login";
import { AVoir } from "./views/AVoir";
import { Journal } from "./views/Journal";
import { Chercher } from "./views/Chercher";
import { Moi } from "./views/Moi";
import { ProductionView } from "./views/Production";
import { ProductionForm } from "./views/ProductionForm";
import { PersonView } from "./views/Person";
import { VenueView } from "./views/Venue";
import { Invite } from "./views/Invite";
import { Admin } from "./views/Admin";

export function App() {
  const me = createMemo(() => api.me());
  setUnauthorizedHandler(() => refresh(me));
  const path = () => route().path;

  return (
    <Errored fallback={(err, reset) => <ErrorPage error={err()} retry={reset} />}>
      <Loading fallback={<p class="loading center-page">Chargement…</p>}>
        <Switch>
          <Match when={match("/invitation/:token", path())}>
            {(p) => <Invite token={p().token} onSignedIn={() => refresh(me)} />}
          </Match>
          <Match when={!me()}>
            <Login onSignedIn={() => refresh(me)} />
          </Match>
          <Match when={me()}>
            {(m) => (
              <>
                <Switch fallback={<NotFound />}>
                  <Match when={path() === "/"}><AVoir name={m().name} /></Match>
                  <Match when={path() === "/journal"}><Journal /></Match>
                  <Match when={path() === "/chercher"}><Chercher /></Match>
                  <Match when={path() === "/moi"}><Moi onSignedOut={() => refresh(me)} /></Match>
                  <Match when={path() === "/spectacles/nouveau"}>
                    <ProductionForm importURL={route().query.get("import") ?? undefined} title={route().query.get("titre") ?? ""} />
                  </Match>
                  <Match when={match("/spectacles/:id/modifier", path())}>{(p) => <ProductionForm id={Number(p().id)} />}</Match>
                  <Match when={match("/spectacles/:id", path())}>{(p) => <ProductionView id={Number(p().id)} />}</Match>
                  <Match when={match("/personnes/:id", path())}>{(p) => <PersonView id={Number(p().id)} />}</Match>
                  <Match when={match("/lieux/:id", path())}>{(p) => <VenueView id={Number(p().id)} />}</Match>
                  <Match when={path() === "/admin" && m().role === "admin"}><Admin /></Match>
                </Switch>
                <TabBar />
              </>
            )}
          </Match>
        </Switch>
      </Loading>
    </Errored>
  );
}

function NotFound() {
  return (
    <main class="page">
      <div class="empty-state">
        <p class="title">Page introuvable</p>
        <a class="btn" href="/">Retour à l'accueil</a>
      </div>
    </main>
  );
}

export function ErrorPage(props: { error: unknown; retry: () => void }) {
  return (
    <main class="page">
      <div class="empty-state">
        <p class="title">Quelque chose a échoué</p>
        <p>{props.error instanceof Error ? props.error.message : String(props.error)}</p>
        <button class="btn" onClick={() => props.retry()}>Réessayer</button>
      </div>
    </main>
  );
}

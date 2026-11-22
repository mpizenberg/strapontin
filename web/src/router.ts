import { createSignal } from "solid-js";

function current() {
  return { path: location.pathname, query: new URLSearchParams(location.search) };
}

const [loc, setLoc] = createSignal(current());

/** The current location; reading it tracks navigation. */
export const route = loc;

export function navigate(to: string, opts: { replace?: boolean } = {}) {
  // inApp marks entries that have one of our pages before them, for back().
  if (opts.replace) history.replaceState(history.state, "", to);
  else history.pushState({ inApp: true }, "", to);
  setLoc(current());
  window.scrollTo(0, 0);
}

/** Goes back when the previous page is ours, else to a fallback. */
export function back(fallback = "/") {
  if (history.state?.inApp) history.back();
  else navigate(fallback, { replace: true });
}

/** Matches "/spectacles/:id" against a path and returns the parameters. */
export function match(pattern: string, path: string): Record<string, string> | undefined {
  const p = pattern.split("/");
  const s = path.replace(/\/+$/, "").split("/");
  if (p.length !== s.length) return undefined;
  const params: Record<string, string> = {};
  for (let i = 0; i < p.length; i++) {
    if (p[i].startsWith(":")) params[p[i].slice(1)] = decodeURIComponent(s[i]);
    else if (p[i] !== s[i]) return undefined;
  }
  return params;
}

/** Turns clicks on same-origin links into client-side navigation. */
export function startRouter() {
  window.addEventListener("popstate", () => setLoc(current()));
  document.addEventListener("click", (e) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const a = (e.target as Element).closest("a");
    const href = a?.getAttribute("href");
    if (!a || !href || !href.startsWith("/") || href.startsWith("/api/") || a.target) return;
    e.preventDefault();
    navigate(href);
  });
}

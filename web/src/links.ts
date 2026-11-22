import { navigate } from "./router";

/** The first web address in a text: a pasted link, or a link shared with some words around it. */
export function findURL(text: string): string | undefined {
  return text.match(/https?:\/\/\S+/i)?.[0] ?? (/^www\.\S+$/i.test(text.trim()) ? text.trim() : undefined);
}

/** Where a page is read and reviewed before it becomes a production. */
export const importPath = (url: string) => `/spectacles/nouveau?import=${encodeURIComponent(url)}`;

/**
 * Android's share sheet opens /chercher?titre=…&texte=…&url=… (see the
 * manifest). Many apps put the link in the text, so both are searched.
 */
export function takeSharedLink() {
  const q = new URLSearchParams(location.search);
  if (location.pathname !== "/chercher" || !(q.has("url") || q.has("texte"))) return;
  const url = findURL(`${q.get("url") ?? ""} ${q.get("texte") ?? ""}`);
  navigate(url ? importPath(url) : `/chercher?q=${encodeURIComponent(q.get("titre") ?? q.get("texte") ?? "")}`, { replace: true });
}

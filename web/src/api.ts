import type {
  AgendaEvent, Candidate, Favorite, ImportResult, Me, PersonRef, Outing, OutingInput, Person, Production, ProductionInput, Role, SearchResult, Status, Venue, WikidataEntity,
} from "./types";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

let onUnauthorized = () => {};

// The app shell registers how to react when the session has expired.
export function setUnauthorizedHandler(f: () => void) {
  onUnauthorized = f;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });
  if (res.status === 401 && path !== "/api/login" && path !== "/api/me") onUnauthorized();
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new ApiError(res.status, data.error ?? "Erreur réseau.");
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

const get = <T>(path: string) => request<T>("GET", path);
const qs = (params: Record<string, string>) => new URLSearchParams(params).toString();

export const api = {
  // Resolves to null when nobody is signed in.
  me: () => get<Me>("/api/me").catch((e) => (e instanceof ApiError && e.status === 401 ? null : Promise.reject(e))),
  login: (login: string, password: string) => request<Me>("POST", "/api/login", { login, password }),
  logout: () => request<void>("POST", "/api/logout"),
  roles: () => get<Role[]>("/api/roles"),

  importURL: (url: string) => request<ImportResult>("POST", "/api/import", { url }),
  parseCredits: (text: string) =>
    request<{ person: PersonRef; role: string; character: string; candidates?: Candidate[] }[]>("POST", "/api/credits/parse", { text }),
  agendas: (q: string, source: "openagenda" | "tourisme") => get<AgendaEvent[]>(`/api/sources/search?${qs({ q, source })}`),
  wikidata: (q: string) => get<WikidataEntity[]>(`/api/wikidata/search?${qs({ q })}`),
  linkWikidata: (personID: number, id: string) => request<void>("PUT", `/api/people/${personID}/wikidata`, { id }),
  search: (q: string, kind = "") => get<SearchResult>(`/api/search?${qs({ q, kind })}`),
  production: (id: number) => get<Production>(`/api/productions/${id}`),
  createProduction: (input: ProductionInput & { status?: Status; snapshot_id?: number; image?: string }) =>
    request<{ id: number }>("POST", "/api/productions", input),
  updateProduction: (id: number, input: ProductionInput) => request<{ id: number }>("PUT", `/api/productions/${id}`, input),
  person: (id: number) => get<Person>(`/api/people/${id}`),
  renamePerson: (id: number, name: string) => request<void>("PUT", `/api/people/${id}`, { name }),
  venue: (id: number) => get<Venue>(`/api/venues/${id}`),
  updateVenue: (id: number, v: { name: string; city: string; address: string }) =>
    request<void>("PUT", `/api/venues/${id}`, v),

  outings: (status: Status) => get<Outing[]>(`/api/outings?${qs({ status })}`),
  createOuting: (input: OutingInput) => request<{ id: number }>("POST", "/api/outings", input),
  updateOuting: (id: number, input: OutingInput) => request<void>("PUT", `/api/outings/${id}`, input),
  deleteOuting: (id: number) => request<void>("DELETE", `/api/outings/${id}`),

  favorites: () => get<Favorite[]>("/api/favorites"),
  setFavorite: (entity: string, id: number, on: boolean) =>
    request<void>(on ? "PUT" : "DELETE", `/api/favorites/${entity}/${id}`),
  setNote: (entity: string, id: number, body: string) => request<void>("PUT", `/api/notes/${entity}/${id}`, { body }),
};

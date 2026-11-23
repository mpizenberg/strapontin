// Mirrors the JSON shapes of internal/api. Keep in sync by hand.

export type Status = "want" | "seen";

export interface Member {
  id: number;
  name: string;
  login: string;
  role: "admin" | "member";
}

export interface Me extends Member {
  created_at: string;
  seen: number;
  wanted: number;
  invites_left: number; // -1: no limit
}

export interface InviteLink {
  url: string;
  expires_at: string;
}

export interface AdminMember {
  id: number;
  name: string;
  login: string;
  role: "admin" | "member";
  invited_by?: string;
  depth: number;
  created_at: string;
  last_seen?: string;
  disabled: boolean;
  outings: number;
  imports: number;
}

export interface Count {
  key: string;
  count: number;
}

export interface Settings {
  max_members: number;
  invite_quota: number;
  ntfy_topic: string;
}

export interface Overview {
  members: number;
  active_7: number;
  active_30: number;
  max_depth: number;
  signups: Count[];
  imports: Count[];
  fetches: Count[];
  login_failed: number;
  db_bytes: number;
  snapshots: number;
  snapshot_bytes: number;
  images: number;
  image_bytes: number;
  last_backup?: string;
  settings: Settings;
}

export interface Role {
  key: string;
  label: string;
}

export interface PersonRef {
  id?: number;
  name: string;
}

export interface WorkRef {
  id?: number;
  title: string;
}

export interface VenueRef {
  id?: number;
  name: string;
  city?: string;
}

export interface Mine {
  status: Status;
  rating?: number;
}

export interface Summary {
  id: number;
  title: string;
  poster?: string;
  venue?: VenueRef;
  starts_on?: string;
  ends_on?: string;
  mine?: Mine;
}

export interface Credit {
  id: number;
  person: PersonRef & { id: number };
  role: string;
  character?: string;
  favorite: boolean;
  seen: number;
}

export interface Run {
  id: number;
  venue: VenueRef & { id: number };
  starts_on?: string;
  ends_on?: string;
}

export interface Outing {
  id: number;
  production_id: number;
  run_id: number | null;
  venue?: VenueRef;
  status: Status;
  date?: string;
  rating?: number;
  comment?: string;
  created_at: string;
  updated_at: string;
  production?: Summary;
}

export interface Production {
  id: number;
  title: string;
  work: WorkRef | null;
  company?: string;
  description?: string;
  url?: string;
  poster?: string;
  credits: Credit[];
  runs: Run[];
  revisions: number;
  created_at: string;
  outings: Outing[];
  others: Summary[];
}

export interface ExternalID {
  scheme: string;
  value: string;
}

export interface Person {
  id: number;
  name: string;
  favorite: boolean;
  note: string;
  external_ids: ExternalID[];
  credits: { role: string; character?: string; production: Summary }[];
  homonyms: Homonym[];
}

export interface Homonym {
  id: number;
  name: string;
  roles: string[];
  productions: number;
}

export interface Venue {
  id: number;
  name: string;
  city: string;
  address: string;
  favorite: boolean;
  note: string;
  productions: Summary[];
}

export interface Favorite {
  entity: "person" | "venue" | "work";
  id: number;
  name: string;
  city?: string;
  roles?: string[];
  productions: number;
}

export interface SearchResult {
  productions: Summary[];
  people: (PersonRef & { id: number })[];
  venues: (VenueRef & { id: number })[];
  works: (WorkRef & { id: number })[];
}

export interface Candidate {
  id: number;
  name: string;
  roles?: string[];
}

// What the production form sends. References without an id create new rows.
// candidates and guessed only come from an import, for the review.
export interface ProductionInput {
  title: string;
  work: WorkRef | null;
  company: string;
  description: string;
  url: string;
  credits: { person: PersonRef; role: string; character: string; candidates?: Candidate[] }[];
  runs: { id?: number; venue: VenueRef; starts_on: string; ends_on: string; guessed?: boolean }[];
}

export interface ImportDraft extends ProductionInput {
  snapshot_id: number;
  image?: string;
  source: "jsonld" | "microdata" | "opengraph" | "html";
}

export interface ImportResult {
  duplicate?: Summary;
  draft?: ImportDraft;
}

export interface OutingInput {
  production_id?: number;
  run_id: number | null;
  status: Status;
  date: string;
  rating: number | null;
  comment: string;
}

/** A show found in an outside agenda; picking it imports its page. */
export interface AgendaEvent {
  title: string;
  venue?: string;
  city?: string;
  starts_on?: string;
  ends_on?: string;
  url: string;
  source: "openagenda" | "tourisme";
}

export interface WikidataEntity {
  id: string;
  label: string;
  description?: string;
}

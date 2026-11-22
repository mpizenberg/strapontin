-- Accounts

CREATE TABLE member (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL,
  login         TEXT NOT NULL UNIQUE,
  password_hash TEXT,
  role          TEXT NOT NULL CHECK (role IN ('admin','member')),
  invited_by    INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at    TEXT NOT NULL,
  last_seen_at  TEXT,
  disabled_at   TEXT
);

CREATE TABLE session (
  token_hash TEXT PRIMARY KEY,
  member_id  INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  user_agent TEXT
);

CREATE TABLE invite (
  token_hash TEXT PRIMARY KEY,
  kind       TEXT NOT NULL CHECK (kind IN ('signup','reset')),
  created_by INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  for_member INTEGER REFERENCES member(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_by    INTEGER REFERENCES member(id) ON DELETE SET NULL,
  used_at    TEXT
);

CREATE TABLE event (
  id        INTEGER PRIMARY KEY,
  member_id INTEGER REFERENCES member(id) ON DELETE SET NULL,
  kind      TEXT NOT NULL,
  at        TEXT NOT NULL
);
CREATE INDEX event_at ON event(at);

CREATE TABLE setting (key TEXT PRIMARY KEY, value TEXT NOT NULL);

-- Shared catalogue

CREATE TABLE person (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  name_key   TEXT NOT NULL,
  created_by INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX person_name_key ON person(name_key);

CREATE TABLE work (
  id         INTEGER PRIMARY KEY,
  title      TEXT NOT NULL,
  created_by INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE venue (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  city       TEXT,
  address    TEXT,
  created_by INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE production (
  id          INTEGER PRIMARY KEY,
  title       TEXT NOT NULL,
  work_id     INTEGER REFERENCES work(id),
  company     TEXT,
  description TEXT,
  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL
);

-- A production played at one venue between two dates.
CREATE TABLE run (
  id            INTEGER PRIMARY KEY,
  production_id INTEGER NOT NULL REFERENCES production(id) ON DELETE CASCADE,
  venue_id      INTEGER NOT NULL REFERENCES venue(id),
  starts_on     TEXT,
  ends_on       TEXT
);
CREATE INDEX run_production ON run(production_id);

CREATE TABLE credit (
  id            INTEGER PRIMARY KEY,
  production_id INTEGER REFERENCES production(id) ON DELETE CASCADE,
  work_id       INTEGER REFERENCES work(id) ON DELETE CASCADE,
  person_id     INTEGER NOT NULL REFERENCES person(id),
  role          TEXT NOT NULL,
  character     TEXT,
  position      INTEGER NOT NULL DEFAULT 0,
  CHECK ((production_id IS NULL) <> (work_id IS NULL))
);
CREATE INDEX credit_production ON credit(production_id);
CREATE INDEX credit_person ON credit(person_id);

CREATE TABLE external_id (
  entity    TEXT NOT NULL CHECK (entity IN ('person','work','venue','production')),
  entity_id INTEGER NOT NULL,
  scheme    TEXT NOT NULL,
  value     TEXT NOT NULL,
  PRIMARY KEY (scheme, value)
);
CREATE INDEX external_id_entity ON external_id(entity, entity_id);

CREATE TABLE snapshot (
  id                INTEGER PRIMARY KEY,
  url               TEXT NOT NULL,
  fetched_at        TEXT NOT NULL,
  fetched_by        INTEGER REFERENCES member(id) ON DELETE SET NULL,
  http_status       INTEGER NOT NULL,
  content_type      TEXT,
  body              BLOB NOT NULL,
  sha256            TEXT NOT NULL,
  extractor_version INTEGER NOT NULL,
  extracted         TEXT,
  production_id     INTEGER REFERENCES production(id) ON DELETE SET NULL
);
CREATE INDEX snapshot_url ON snapshot(url);

CREATE TABLE image (
  id            INTEGER PRIMARY KEY,
  production_id INTEGER NOT NULL REFERENCES production(id) ON DELETE CASCADE,
  source_url    TEXT NOT NULL,
  content_type  TEXT NOT NULL,
  bytes         BLOB NOT NULL,
  fetched_at    TEXT NOT NULL
);
CREATE INDEX image_production ON image(production_id);

CREATE TABLE revision (
  id        INTEGER PRIMARY KEY,
  entity    TEXT NOT NULL,
  entity_id INTEGER NOT NULL,
  member_id INTEGER REFERENCES member(id) ON DELETE SET NULL,
  at        TEXT NOT NULL,
  before    TEXT NOT NULL
);
CREATE INDEX revision_entity ON revision(entity, entity_id);

-- Personal layer

CREATE TABLE outing (
  id            INTEGER PRIMARY KEY,
  member_id     INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  production_id INTEGER NOT NULL REFERENCES production(id),
  run_id        INTEGER REFERENCES run(id) ON DELETE SET NULL,
  status        TEXT NOT NULL CHECK (status IN ('want','seen')),
  date          TEXT,
  rating        INTEGER CHECK (rating BETWEEN 1 AND 5),
  comment       TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE INDEX outing_member ON outing(member_id, status);
CREATE INDEX outing_production ON outing(production_id);

CREATE TABLE favorite (
  member_id  INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  entity     TEXT NOT NULL CHECK (entity IN ('person','work','venue')),
  entity_id  INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (member_id, entity, entity_id)
);

CREATE TABLE note (
  member_id  INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  entity     TEXT NOT NULL,
  entity_id  INTEGER NOT NULL,
  body       TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (member_id, entity, entity_id)
);

-- Full-text search over the catalogue, kept in sync by triggers.

CREATE VIRTUAL TABLE search USING fts5(
  entity UNINDEXED, entity_id UNINDEXED, text,
  tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER production_ai AFTER INSERT ON production BEGIN
  INSERT INTO search(entity, entity_id, text) VALUES ('production', new.id, new.title || ' ' || coalesce(new.company, ''));
END;
CREATE TRIGGER production_au AFTER UPDATE OF title, company ON production BEGIN
  UPDATE search SET text = new.title || ' ' || coalesce(new.company, '') WHERE entity = 'production' AND entity_id = new.id;
END;
CREATE TRIGGER production_ad AFTER DELETE ON production BEGIN
  DELETE FROM search WHERE entity = 'production' AND entity_id = old.id;
END;

CREATE TRIGGER person_ai AFTER INSERT ON person BEGIN
  INSERT INTO search(entity, entity_id, text) VALUES ('person', new.id, new.name);
END;
CREATE TRIGGER person_au AFTER UPDATE OF name ON person BEGIN
  UPDATE search SET text = new.name WHERE entity = 'person' AND entity_id = new.id;
END;
CREATE TRIGGER person_ad AFTER DELETE ON person BEGIN
  DELETE FROM search WHERE entity = 'person' AND entity_id = old.id;
END;

CREATE TRIGGER work_ai AFTER INSERT ON work BEGIN
  INSERT INTO search(entity, entity_id, text) VALUES ('work', new.id, new.title);
END;
CREATE TRIGGER work_au AFTER UPDATE OF title ON work BEGIN
  UPDATE search SET text = new.title WHERE entity = 'work' AND entity_id = new.id;
END;
CREATE TRIGGER work_ad AFTER DELETE ON work BEGIN
  DELETE FROM search WHERE entity = 'work' AND entity_id = old.id;
END;

CREATE TRIGGER venue_ai AFTER INSERT ON venue BEGIN
  INSERT INTO search(entity, entity_id, text) VALUES ('venue', new.id, new.name || ' ' || coalesce(new.city, ''));
END;
CREATE TRIGGER venue_au AFTER UPDATE OF name, city ON venue BEGIN
  UPDATE search SET text = new.name || ' ' || coalesce(new.city, '') WHERE entity = 'venue' AND entity_id = new.id;
END;
CREATE TRIGGER venue_ad AFTER DELETE ON venue BEGIN
  DELETE FROM search WHERE entity = 'venue' AND entity_id = old.id;
END;

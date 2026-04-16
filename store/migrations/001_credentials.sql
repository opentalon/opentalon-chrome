-- Browser session credentials stored per entity per user-chosen name.
-- entity_id maps to the actor ID from the OpenTalon context.
-- name is a unique user-specified label (e.g. "linkedin-work", "linkedin-personal").
CREATE TABLE browser_credentials (
  entity_id  TEXT NOT NULL,
  name       TEXT NOT NULL,
  cookies    TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (entity_id, name)
);
CREATE INDEX IF NOT EXISTS idx_browser_credentials_entity ON browser_credentials(entity_id);

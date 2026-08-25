CREATE TABLE batches (
  id         INTEGER PRIMARY KEY,
  created_at TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT 'web'
);

CREATE TABLE tasks (
  id           INTEGER PRIMARY KEY,
  topic        TEXT    NOT NULL,
  title        TEXT    NOT NULL,
  note         TEXT    NOT NULL DEFAULT '',
  due_date     TEXT,
  assignee     TEXT,
  priority     INTEGER NOT NULL DEFAULT 0,
  status       TEXT    NOT NULL DEFAULT 'open',
  position     REAL    NOT NULL,
  batch_id     INTEGER REFERENCES batches(id) ON DELETE SET NULL,
  raw          TEXT    NOT NULL DEFAULT '',
  created_at   TEXT    NOT NULL,
  completed_at TEXT
);

CREATE TABLE tags (
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  tag     TEXT    NOT NULL,
  PRIMARY KEY (task_id, tag)
);

CREATE INDEX idx_tasks_status_pos ON tasks(status, position);
CREATE INDEX idx_tasks_due        ON tasks(due_date) WHERE status = 'open';
CREATE INDEX idx_tasks_topic      ON tasks(topic);
CREATE INDEX idx_tasks_batch      ON tasks(batch_id);
CREATE INDEX idx_tags_tag         ON tags(tag);

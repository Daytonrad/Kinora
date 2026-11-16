// Package store is Kinora's SQLite persistence layer.
//
// Jellyfin problem it addresses: large libraries make the UI crawl while a scan
// is running, and backups/migrations are awkward. Kinora uses SQLite in WAL mode
// with short write transactions (readers never block on the scanner), a
// versioned migration list, and a one-click consistent backup via VACUUM INTO.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ErrNotFound is returned when a row does not exist (or is hidden by the rating filter).
var ErrNotFound = errors.New("nem található")

// Store wraps the database handle.
type Store struct {
	DB   *sql.DB
	path string

	mu        sync.RWMutex
	overrides map[string]string // settings forced by environment variables
}

// Open opens (and migrates) the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=10000&_foreign_keys=on&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("adatbázis megnyitása: %w", err)
	}
	s := &Store{DB: db, path: path, overrides: map[string]string{}}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migráció: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

func now() int64 { return time.Now().Unix() }

// migrations are applied in order; never edit an existing entry, only append.
var migrations = []string{
	// 1: initial schema
	`
CREATE TABLE users(
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  pass_hash TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE profiles(
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  color TEXT NOT NULL DEFAULT '#3e9bd6',
  pin_hash TEXT,
  is_kids INTEGER NOT NULL DEFAULT 0,
  max_rating_level INTEGER,
  sub_langs TEXT NOT NULL DEFAULT '',
  audio_langs TEXT NOT NULL DEFAULT '',
  sub_mode TEXT NOT NULL DEFAULT 'auto',
  autoplay_next INTEGER NOT NULL DEFAULT 1,
  max_bitrate INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX profiles_user ON profiles(user_id);
CREATE TABLE sessions(
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  profile_id INTEGER REFERENCES profiles(id) ON DELETE SET NULL,
  expires_at INTEGER NOT NULL
);
CREATE TABLE invites(
  code TEXT PRIMARY KEY,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  note TEXT NOT NULL DEFAULT '',
  uses_left INTEGER NOT NULL,
  expires_at INTEGER,
  created_at INTEGER NOT NULL
);
CREATE TABLE libraries(
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('movies','shows','mixed')),
  last_scan_at INTEGER,
  created_at INTEGER NOT NULL
);
CREATE TABLE items(
  id INTEGER PRIMARY KEY,
  library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('movie','series','season','episode')),
  parent_id INTEGER,
  series_id INTEGER,
  key TEXT NOT NULL,
  title TEXT NOT NULL,
  sort_title TEXT NOT NULL,
  year INTEGER,
  season_num INTEGER,
  episode_num INTEGER,
  episode_end INTEGER,
  overview TEXT NOT NULL DEFAULT '',
  content_rating TEXT NOT NULL DEFAULT '',
  rating_level INTEGER,
  community_rating REAL,
  runtime_sec INTEGER NOT NULL DEFAULT 0,
  path TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  mtime INTEGER NOT NULL DEFAULT 0,
  container TEXT NOT NULL DEFAULT '',
  video_codec TEXT NOT NULL DEFAULT '',
  audio_codec TEXT NOT NULL DEFAULT '',
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  bitrate INTEGER NOT NULL DEFAULT 0,
  hdr TEXT NOT NULL DEFAULT '',
  credits_start REAL,
  tmdb_id INTEGER,
  has_poster INTEGER NOT NULL DEFAULT 0,
  has_backdrop INTEGER NOT NULL DEFAULT 0,
  missing INTEGER NOT NULL DEFAULT 0,
  sidecar_sig TEXT NOT NULL DEFAULT '',
  added_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(library_id, key)
);
CREATE INDEX items_lib_kind_sort ON items(library_id, kind, sort_title);
CREATE INDEX items_parent ON items(parent_id);
CREATE INDEX items_series_ep ON items(series_id, season_num, episode_num);
CREATE INDEX items_added ON items(added_at DESC);
CREATE INDEX items_path ON items(path);
CREATE TABLE media_streams(
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  idx INTEGER NOT NULL,
  type TEXT NOT NULL,
  type_idx INTEGER NOT NULL,
  codec TEXT NOT NULL DEFAULT '',
  lang TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  channels INTEGER NOT NULL DEFAULT 0,
  is_default INTEGER NOT NULL DEFAULT 0,
  is_forced INTEGER NOT NULL DEFAULT 0,
  external_path TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(item_id, type, type_idx)
);
CREATE TABLE chapters(
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  start REAL NOT NULL,
  title TEXT NOT NULL DEFAULT ''
);
CREATE INDEX chapters_item ON chapters(item_id);
CREATE TABLE item_genres(
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  genre TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY(item_id, genre)
);
CREATE INDEX item_genres_genre ON item_genres(genre);
CREATE TABLE item_tags(
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  tag TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY(item_id, tag)
);
CREATE INDEX item_tags_tag ON item_tags(tag);
CREATE TABLE collections(
  id INTEGER PRIMARY KEY,
  profile_id INTEGER REFERENCES profiles(id) ON DELETE CASCADE,
  owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('manual','smart')),
  rules TEXT NOT NULL DEFAULT '',
  pinned INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE collection_items(
  collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  position INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(collection_id, item_id)
);
CREATE INDEX collection_items_item ON collection_items(item_id);
CREATE TABLE user_data(
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  position REAL NOT NULL DEFAULT 0,
  played INTEGER NOT NULL DEFAULT 0,
  play_count INTEGER NOT NULL DEFAULT 0,
  last_played_at INTEGER,
  favorite INTEGER NOT NULL DEFAULT 0,
  stars INTEGER CHECK(stars BETWEEN 1 AND 5),
  hidden INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(profile_id, item_id)
);
CREATE INDEX user_data_item ON user_data(item_id);
CREATE INDEX user_data_last ON user_data(profile_id, last_played_at DESC);
CREATE TABLE settings(
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`,
}

func (s *Store) migrate() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_version(version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var v int
	err := s.DB.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.DB.Exec(`INSERT INTO schema_version(version) VALUES(0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migráció %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`UPDATE schema_version SET version=?`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion returns the applied migration count.
func (s *Store) SchemaVersion() int {
	var v int
	s.DB.QueryRow(`SELECT version FROM schema_version`).Scan(&v)
	return v
}

// ---------------------------------------------------------------- settings

// SettingDefaults are used when a key is not stored.
var SettingDefaults = map[string]string{
	"server_name":       "Kinora",
	"tmdb_key":          "",
	"metadata_lang":     "hu-HU",
	"encoder":           "auto",
	"public_url":        "",
	"scan_interval_min": "15",
}

// SetOverride forces a setting from the environment (shown as locked in the admin UI).
func (s *Store) SetOverride(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrides[key] = value
}

// Overridden reports whether key is forced by an environment variable.
func (s *Store) Overridden(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.overrides[key]
	return ok
}

// Setting returns the effective value: env override > stored > default.
func (s *Store) Setting(key string) string {
	s.mu.RLock()
	if v, ok := s.overrides[key]; ok {
		s.mu.RUnlock()
		return v
	}
	s.mu.RUnlock()
	var v string
	if err := s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return SettingDefaults[key]
	}
	return v
}

// SettingInt returns an integer setting, falling back to def.
func (s *Store) SettingInt(key string, def int) int {
	n, err := strconv.Atoi(s.Setting(key))
	if err != nil {
		return def
	}
	return n
}

// SetSetting stores a value.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---------------------------------------------------------------- backup

// Backup writes a consistent copy of the live database to dst using VACUUM INTO.
func (s *Store) Backup(ctx context.Context, dst string) error {
	os.Remove(dst)
	_, err := s.DB.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}

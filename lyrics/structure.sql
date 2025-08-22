CREATE TABLE tracks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT,
  name_lower TEXT,
  artist_name TEXT,
  artist_name_lower TEXT,
  album_name TEXT,
  album_name_lower TEXT,
  duration FLOAT,
  last_lyrics_id INTEGER,
  created_at DATETIME,
  updated_at DATETIME,
  FOREIGN KEY (last_lyrics_id) REFERENCES lyrics (id),
  UNIQUE(name_lower, artist_name_lower, album_name_lower, duration)
)

CREATE TABLE sqlite_sequence(name,seq)
CREATE TABLE lyrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  plain_lyrics TEXT,
  synced_lyrics TEXT,
  track_id INTEGER,
  has_plain_lyrics BOOLEAN,
  has_synced_lyrics BOOLEAN,
  instrumental BOOLEAN,
  source TEXT,
  created_at DATETIME,
  updated_at DATETIME,
  FOREIGN KEY (track_id) REFERENCES tracks (id)
)
CREATE TRIGGER set_tracks_last_lyrics_id
AFTER INSERT ON lyrics
BEGIN
  UPDATE tracks SET last_lyrics_id = NEW.id WHERE tracks.id = NEW.track_id;
END
CREATE VIRTUAL TABLE tracks_fts USING fts5(
  name_lower,
  album_name_lower,
  artist_name_lower,
  content='tracks',
  content_rowid='id'
)
CREATE TABLE 'tracks_fts_data'(id INTEGER PRIMARY KEY, block BLOB)
CREATE TABLE 'tracks_fts_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID
CREATE TABLE 'tracks_fts_docsize'(id INTEGER PRIMARY KEY, sz BLOB)
CREATE TABLE 'tracks_fts_config'(k PRIMARY KEY, v) WITHOUT ROWID
CREATE TRIGGER tracks_ai AFTER INSERT ON tracks
BEGIN
  INSERT INTO tracks_fts (rowid, name_lower, album_name_lower, artist_name_lower)
  VALUES (new.id, new.name_lower, new.album_name_lower, new.artist_name_lower);
END
CREATE TRIGGER tracks_au AFTER UPDATE ON tracks
BEGIN
  INSERT INTO tracks_fts(tracks_fts, rowid, name_lower, album_name_lower, artist_name_lower)
  VALUES('delete', old.id, old.name_lower, old.album_name_lower, old.artist_name_lower);
  INSERT INTO tracks_fts (rowid, name_lower, album_name_lower, artist_name_lower)
  VALUES (new.id, new.name_lower, new.album_name_lower, new.artist_name_lower);
END
CREATE TRIGGER tracks_ad AFTER DELETE ON tracks
BEGIN
  INSERT INTO tracks_fts(tracks_fts, rowid, name_lower, album_name_lower, artist_name_lower)
  VALUES('delete', old.id, old.name_lower, old.album_name_lower, old.artist_name_lower);
END
CREATE INDEX idx_tracks_name_lower ON tracks (name_lower)
CREATE INDEX idx_tracks_artist_name_lower ON tracks (artist_name_lower)
CREATE INDEX idx_tracks_album_name_lower ON tracks (album_name_lower)
CREATE INDEX idx_tracks_duration ON tracks (duration)
CREATE INDEX idx_tracks_last_lyrics_id ON tracks (last_lyrics_id)
CREATE INDEX idx_lyrics_track_id ON lyrics (track_id)
CREATE INDEX idx_lyrics_has_plain_lyrics ON lyrics (has_plain_lyrics)
CREATE INDEX idx_lyrics_has_synced_lyrics ON lyrics (has_synced_lyrics)
CREATE INDEX idx_lyrics_source ON lyrics (source)
CREATE TABLE _litestream_seq (id INTEGER PRIMARY KEY, seq INTEGER)
CREATE TABLE _litestream_lock (id INTEGER)
CREATE INDEX idx_lyrics_created_at ON lyrics (created_at)
CREATE TABLE flags (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  lyrics_id INTEGER,
  content TEXT,
  created_at DATETIME,
  FOREIGN KEY (lyrics_id) REFERENCES lyrics (id)
)
CREATE TABLE missing_tracks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT,
  name_lower TEXT,
  artist_name TEXT,
  artist_name_lower TEXT,
  album_name TEXT,
  album_name_lower TEXT,
  duration FLOAT,
  created_at DATETIME,
  updated_at DATETIME,
  UNIQUE(name_lower, artist_name_lower, album_name_lower, duration)
)

CREATE INDEX idx_missing_tracks_name_lower ON missing_tracks (name_lower)
CREATE INDEX idx_missing_tracks_artist_name_lower ON missing_tracks (artist_name_lower)
CREATE INDEX idx_missing_tracks_album_name_lower ON missing_tracks (album_name_lower)
CREATE INDEX idx_missing_tracks_duration ON missing_tracks (duration)
CREATE INDEX idx_missing_tracks_created_at ON missing_tracks (created_at)

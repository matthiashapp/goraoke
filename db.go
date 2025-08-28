package main

import (
	"database/sql"
	"fmt"
	"strings"
)

type Lyric struct {
	ID              sql.NullInt64
	NameLower       sql.NullString
	DurationSeconds sql.NullFloat64
	PlainLyrics     sql.NullString
	SyncedLyrics    sql.NullString
	HasPlain        sql.NullBool
	HasSynced       sql.NullBool
}

// queryLyrics opens the sqlite database at dbPath, executes the parameterized query,
// and returns either synced or plain lyrics (depending on wantSynced). It returns an
// error if the DB can't be opened or the query fails or returns no rows.
func queryLyrics(db *sql.DB, artistLower, nameLower string, duration int) (*Lyric, error) {
	nameLower = strings.TrimSpace(nameLower)
	if idx := strings.Index(nameLower, "("); idx != -1 {
		nameLower = nameLower[:idx]
	}
	nameLower = strings.TrimSpace(nameLower)
	const q = `SELECT
  t.id,
  t.name_lower,
  t.duration,
  l.plain_lyrics,
  l.synced_lyrics,
  l.has_plain_lyrics,
  l.has_synced_lyrics
FROM
  tracks t
JOIN lyrics l ON l.track_id = t.id
WHERE
  artist_name_lower = ? AND
  name_lower = ? AND
  duration = ?
LIMIT 1;`

	it := &Lyric{}
	err := db.
		QueryRow(q, artistLower, nameLower, duration).
		Scan(
			&it.ID,
			&it.NameLower,
			&it.DurationSeconds,
			&it.PlainLyrics,
			&it.SyncedLyrics,
			&it.HasPlain,
			&it.HasSynced,
		)
	if err != nil {
		if err == sql.ErrNoRows {
			// try to find without duration
			const q2 = `SELECT
  t.id,
  t.name_lower,
  t.duration,
  l.plain_lyrics,
  l.synced_lyrics,
  l.has_plain_lyrics,
  l.has_synced_lyrics
FROM
  tracks t
JOIN lyrics l ON l.track_id = t.id
WHERE
  artist_name_lower = ? AND
  name_lower = ?
LIMIT 1;`

			err = db.
				QueryRow(q2, artistLower, nameLower).
				Scan(
					&it.ID,
					&it.NameLower,
					&it.DurationSeconds,
					&it.PlainLyrics,
					&it.SyncedLyrics,
					&it.HasPlain,
					&it.HasSynced,
				)
		}
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
	}
	return it, nil
}

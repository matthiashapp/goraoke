SELECT
  t.id,
  t.name_lower,
  t.duration,
  l.plain_lyrics,
  l.synced_lyrics
FROM
  tracks t
JOIN lyrics l ON l.track_id = t.id
WHERE
  artist_name_lower = 'fall out boy'
LIMIT 1;


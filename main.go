package main

import (
	"database/sql"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	db, err := sql.Open("sqlite", "lyrics/music.sqlite3")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	m := newModel(db)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		log.Fatal(err)
	}
}

// === Bubble Tea model ===
type model struct {
	db       *sql.DB
	info     TrackInfo
	lyric    *Lyric
	synced   []SyncedLine // ordered by Seconds ascending
	progress progress.Model
	err      error
	// caches to avoid re-querying & re-parsing lyrics when switching back to tracks
	lyricCache  map[string]*Lyric
	syncedCache map[string][]SyncedLine
}

func newModel(db *sql.DB) model {
	p := progress.New(progress.WithDefaultGradient())
	p.Width = 60
	return model{db: db, progress: p, lyricCache: make(map[string]*Lyric), syncedCache: make(map[string][]SyncedLine)}
}

// messages
type tickMsg time.Time
type trackMsg TrackInfo
type lyricMsg *Lyric
type errMsg struct{ err error }

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchTrackCmd(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchTrackCmd() tea.Cmd {
	return func() tea.Msg {
		info, err := QueryCmus()
		if err != nil {
			return errMsg{err}
		}
		return trackMsg(info)
	}
}

func fetchLyricCmd(db *sql.DB, artist, title string, duration int) tea.Cmd {
	return func() tea.Msg {
		l, err := queryLyrics(db, strings.ToLower(artist), strings.ToLower(title), duration)
		if err != nil {
			if err == sql.ErrNoRows {
				return lyricMsg(nil)
			}
			return errMsg{err}
		}
		return lyricMsg(l)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		// Poll position slightly more often than track info; quick command for only position would be better.
		return m, tea.Batch(fetchTrackCmd(), tickCmd())
	case trackMsg:
		oldTitle := m.info.Title
		oldArtist := m.info.Artist
		m.info = TrackInfo(msg)
		// Track changed -> refetch lyrics
		if m.info.Title != oldTitle || m.info.Artist != oldArtist {
			m.lyric = nil
			m.synced = nil
			// check cache first
			key := cacheKey(m.info.Artist, m.info.Title)
			if l, ok := m.lyricCache[key]; ok {
				m.lyric = l
				m.synced = m.syncedCache[key]
				return m, nil
			}
			return m, fetchLyricCmd(m.db, m.info.Artist, m.info.Title, m.info.Duration)
		}
	case lyricMsg:
		m.lyric = (*Lyric)(msg)
		if m.lyric != nil && m.lyric.SyncedLyrics.Valid {
			key := cacheKey(m.info.Artist, m.info.Title)
			// parse only if not already cached
			if existing, ok := m.syncedCache[key]; ok {
				m.synced = existing
			} else {
				synced, _ := ParseSyncedLyrics(m.lyric.SyncedLyrics.String)
				m.synced = synced
				m.syncedCache[key] = synced
			}
			// cache lyric object itself (even if plain only)
			if m.lyric != nil {
				m.lyricCache[key] = m.lyric
			}
		} else if m.lyric != nil { // plain lyrics only
			key := cacheKey(m.info.Artist, m.info.Title)
			m.lyricCache[key] = m.lyric
		}
	case errMsg:
		m.err = msg.err
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return m, tea.Quit
		}
	}
	// progress bar is purely derived from current/total; no internal state updates needed
	return m, nil
}

var (
	titleStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	artistStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	lyricUpcomingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	lyricPastStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	lyricCurrentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
)

// currentLyricIndex returns index of current line (largest Seconds <= current playback) and ok flag.
func (m model) currentLyricIndex() (int, bool) {
	if len(m.synced) == 0 {
		return 0, false
	}
	cur := m.info.Current
	idx := -1
	for i, l := range m.synced { // lyrics list is ordered
		if l.Seconds <= cur {
			idx = i
		} else {
			break
		}
	}
	if idx == -1 {
		return 0, false
	}
	return idx, true
}

// lyricWindow returns a window of lines around current (3 before, 3 after) formatted.
func (m model) lyricWindow() string {
	if len(m.synced) == 0 {
		// Plain lyrics fallback
		if m.lyric != nil && m.lyric.PlainLyrics.Valid {
			lines := strings.Split(m.lyric.PlainLyrics.String, "\n")
			if len(lines) > 7 {
				lines = lines[:7]
			}
			out := make([]string, len(lines))
			for i, l := range lines {
				if i == 0 {
					out[i] = lyricCurrentStyle.Render(l)
				} else {
					out[i] = lyricUpcomingStyle.Render(l)
				}
			}
			return strings.Join(out, "\n")
		}
		return ""
	}
	idx, ok := m.currentLyricIndex()
	if !ok { // before first line
		first := 0
		end := min(7, len(m.synced))
		out := make([]string, 0, end-first)
		for i := first; i < end; i++ {
			out = append(out, lyricUpcomingStyle.Render(m.synced[i].Text))
		}
		return strings.Join(out, "\n")
	}
	start := max(idx-3, 0)
	end := min(
		// exclusive
		idx+4, len(m.synced))
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := m.synced[i].Text
		switch {
		case i == idx:
			out = append(out, lyricCurrentStyle.Render(line))
		case i < idx:
			out = append(out, lyricPastStyle.Render(line))
		default:
			out = append(out, lyricUpcomingStyle.Render(line))
		}
	}
	return strings.Join(out, "\n")
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if m.info.Duration == 0 {
		return "Waiting for cmus... (q to quit)"
	}
	ratio := float64(m.info.Current) / float64(m.info.Duration)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	bar := m.progress.ViewAs(ratio)
	lyricsBlock := m.lyricWindow()
	if lyricsBlock == "" && m.lyric == nil {
		lyricsBlock = lyricUpcomingStyle.Render("(searching lyrics...)")
	} else if lyricsBlock == "" && m.lyric != nil && len(m.synced) == 0 && !m.lyric.PlainLyrics.Valid {
		lyricsBlock = lyricUpcomingStyle.Render("(no lyrics)")
	}
	meta := fmt.Sprintf("%s - %s", artistStyle.Render(m.info.Artist), titleStyle.Render(m.info.Title))
	ts := fmt.Sprintf("%s / %s", formatTime(m.info.Current), formatTime(m.info.Duration))
	return fmt.Sprintf("%s\n%s\n%s\n%s\n", meta, bar, ts, lyricsBlock)
}

func formatTime(secs int) string {
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

type Lyric struct {
	ID              sql.NullInt64
	NameLower       sql.NullString
	DurationSeconds sql.NullFloat64
	PlainLyrics     sql.NullString
	SyncedLyrics    sql.NullString
}

// queryLyrics opens the sqlite database at dbPath, executes the parameterized query,
// and returns either synced or plain lyrics (depending on wantSynced). It returns an
// error if the DB can't be opened or the query fails or returns no rows.
func queryLyrics(db *sql.DB, artistLower, nameLower string, duration int) (*Lyric, error) {
	const q = `SELECT
  t.id,
  t.name_lower,
  t.duration,
  l.plain_lyrics,
  l.synced_lyrics
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
		Scan(&it.ID, &it.NameLower, &it.DurationSeconds, &it.PlainLyrics, &it.SyncedLyrics)
	if err != nil {
		if err == sql.ErrNoRows {
			// try to find without duration
			const q2 = `SELECT
  t.id,
  t.name_lower,
  t.duration,
  l.plain_lyrics,
  l.synced_lyrics
FROM
  tracks t
JOIN lyrics l ON l.track_id = t.id
WHERE
  artist_name_lower = ? AND
  name_lower = ?
LIMIT 1;`

			err = db.
				QueryRow(q2, artistLower, nameLower).
				Scan(&it.ID, &it.NameLower, &it.DurationSeconds, &it.PlainLyrics, &it.SyncedLyrics)
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

// TrackInfo holds parsed data from `cmus-remote -Q` output.
type TrackInfo struct {
	Artist string
	Title  string
	// Duration in seconds
	Duration int
	// Current position in seconds
	Current int
}

// ParseCmusOutput parses the text produced by `cmus-remote -Q` and returns a TrackInfo.
// It is resilient: missing fields are left zero-valued.
func ParseCmusOutput(output string) (TrackInfo, error) {
	var info TrackInfo
	if strings.TrimSpace(output) == "" {
		return info, fmt.Errorf("empty output")
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Example tag lines: "tag artist Radiohead"
		if strings.HasPrefix(line, "tag artist ") {
			info.Artist = strings.TrimSpace(line[len("tag artist "):])
			continue
		}
		if strings.HasPrefix(line, "tag title ") {
			info.Title = strings.TrimSpace(line[len("tag title "):])
			continue
		}

		// duration <seconds>
		if strings.HasPrefix(line, "duration ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.Atoi(parts[1]); err == nil {
					info.Duration = v
				}
			}
			continue
		}

		// position <seconds>  (the currently played position)
		if strings.HasPrefix(line, "position ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.Atoi(parts[1]); err == nil {
					info.Current = v
				}
			}
			continue
		}
	}

	return info, nil
}

// QueryCmus runs `cmus-remote -Q` and parses the result. Returns an error if the command fails.
func QueryCmus() (TrackInfo, error) {
	out, err := exec.Command("cmus-remote", "-Q").CombinedOutput()
	if err != nil {
		return TrackInfo{}, fmt.Errorf("running cmus-remote: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	return ParseCmusOutput(string(out))
}

// SyncedLine represents a timestamped lyric line.
type SyncedLine struct {
	Seconds int
	Text    string
}

// cacheKey builds a simple case-insensitive key for artist+title lyric caches.
func cacheKey(artist, title string) string {
	return strings.ToLower(strings.TrimSpace(artist)) + "\n" + strings.ToLower(strings.TrimSpace(title))
}

// ParseSyncedLyrics parses basic LRC-style content. Supports multiple timestamps per line
// like: [00:10.00][00:12.50]Some text
// Fractional seconds are truncated to int seconds for now.
func ParseSyncedLyrics(s string) ([]SyncedLine, error) {
	var out []SyncedLine
	lines := strings.Split(s, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// gather timestamps
		var stamps []int
		rest := line
		for strings.HasPrefix(rest, "[") {
			end := strings.Index(rest, "]")
			if end == -1 {
				break
			}
			stamp := rest[1:end]
			parts := strings.Split(stamp, ":")
			if len(parts) == 2 {
				min, err1 := strconv.Atoi(parts[0])
				secPart := parts[1]
				secFrac := strings.SplitN(secPart, ".", 2)
				sec, err2 := strconv.Atoi(secFrac[0])
				if err1 == nil && err2 == nil {
					total := min*60 + sec
					stamps = append(stamps, total)
				}
			}
			rest = strings.TrimSpace(rest[end+1:])
		}
		if len(stamps) == 0 {
			continue
		}
		text := strings.TrimSpace(rest)
		for _, ts := range stamps {
			out = append(out, SyncedLine{Seconds: ts, Text: text})
		}
	}
	// sort
	if len(out) > 1 {
		// simple insertion sort (few lines) to avoid importing sort for tiny benefit
		for i := 1; i < len(out); i++ {
			j := i
			for j > 0 && out[j-1].Seconds > out[j].Seconds {
				out[j-1], out[j] = out[j], out[j-1]
				j--
			}
		}
	}
	return out, nil
}

package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// === Bubble Tea model ===
type model struct {
	db          *sql.DB
	info        TrackInfo
	lyric       *Lyric
	synced      []SyncedLine // ordered by Seconds ascending
	progress    progress.Model
	err         error
	lyricCache  map[string]*Lyric
	syncedCache map[string][]SyncedLine
}

func newModel(db *sql.DB) model {
	p := progress.New(progress.WithDefaultGradient())
	p.Width = 60
	return model{
		db:          db,
		progress:    p,
		lyricCache:  make(map[string]*Lyric),
		syncedCache: make(map[string][]SyncedLine),
	}
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
				synced, err := ParseSyncedLyrics(m.lyric.SyncedLyrics.String)
				if err != nil {
					log.Fatal(err)
				}
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
	if m.lyric == nil {
		lyricsBlock = lyricUpcomingStyle.Render("(no lyrics)")
	}
	meta := fmt.Sprintf(
		"%s - %s",
		artistStyle.Render(m.info.Artist),
		titleStyle.Render(m.info.Title),
	)
	ts := fmt.Sprintf(
		"%s / %s",
		formatTime(m.info.Current),
		formatTime(m.info.Duration),
	)
	return fmt.Sprintf(
		"%s\n%s\n%s\n%s\n",
		meta,
		bar,
		ts,
		lyricsBlock,
	)
}

func formatTime(secs int) string {
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
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

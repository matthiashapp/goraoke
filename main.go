package main

import (
	"database/sql"
	"log"

	tea "github.com/charmbracelet/bubbletea"
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

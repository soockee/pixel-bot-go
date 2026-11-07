package view

import (
	"fmt"
	"time"

	//lint:ignore ST1001 Dot import for concise Tk widget DSL.
	. "modernc.org/tk9.0"
)

// SessionStats shows session and total capture durations.
type SessionStats struct {
	sessionLbl *LabelWidget
	totalLbl   *LabelWidget
}

// NewSessionStats creates the labels at (row, startCol) and (row, startCol+1) of parent.
func NewSessionStats(parent *FrameWidget, row, startCol int) *SessionStats {
	s := &SessionStats{sessionLbl: Label(Width(14)), totalLbl: Label(Width(14))}
	Grid(s.sessionLbl, In(parent), Row(row), Column(startCol), Sticky("w"), Padx("0.2m"))
	Grid(s.totalLbl, In(parent), Row(row), Column(startCol+1), Sticky("w"), Padx("0.2m"))
	s.Set(0, 0)
	return s
}

// Set updates both durations.
func (s *SessionStats) Set(session, total time.Duration) {
	s.sessionLbl.Configure(Txt("Session: " + clock(session)))
	s.totalLbl.Configure(Txt("Total: " + clock(total)))
}

func clock(d time.Duration) string {
	seconds := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

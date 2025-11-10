package view

import (
	"fmt"
	"time"

	//lint:ignore ST1001 Dot import for concise Tk widget DSL.
	. "modernc.org/tk9.0"
)

// SessionStats updates session and total capture durations.
type SessionStats interface {
	SetSession(d time.Duration)
	SetTotal(d time.Duration)
}

type sessionStats struct {
	sessionLbl *LabelWidget
	totalLbl   *LabelWidget
}

// NewSessionStats creates session and total duration labels in a grid layout.
// The session label is placed at (row, startCol) and total label at (row, startCol+1).
// If parent is nil, labels are positioned relative to the App root.
func NewSessionStats(parent *FrameWidget, row, startCol int) SessionStats {
	s := &sessionStats{sessionLbl: Label(Width(14)), totalLbl: Label(Width(14))}
	// Position session label
	if parent != nil {
		Grid(s.sessionLbl, In(parent), Row(row), Column(startCol), Sticky("w"), Padx("0.2m"))
	} else {
		Grid(s.sessionLbl, Row(row), Column(startCol), Sticky("w"), Padx("0.2m"))
	}
	// Position total label
	if parent != nil {
		Grid(s.totalLbl, In(parent), Row(row), Column(startCol+1), Sticky("w"), Padx("0.2m"))
	} else {
		Grid(s.totalLbl, Row(row), Column(startCol+1), Sticky("w"), Padx("0.2m"))
	}
	s.sessionLbl.Configure(Txt("Session: 00:00"))
	s.totalLbl.Configure(Txt("Total: 00:00"))
	return s
}

// SetSession updates the session duration display.
func (s *sessionStats) SetSession(d time.Duration) {
	if s == nil || s.sessionLbl == nil {
		return
	}
	seconds := int(d.Seconds())
	min, sec := seconds/60, seconds%60
	s.sessionLbl.Configure(Txt(fmt.Sprintf("Session: %02d:%02d", min, sec)))
}

// SetTotal updates the total duration display.
func (s *sessionStats) SetTotal(d time.Duration) {
	if s == nil || s.totalLbl == nil {
		return
	}
	seconds := int(d.Seconds())
	min, sec := seconds/60, seconds%60
	s.totalLbl.Configure(Txt(fmt.Sprintf("Total: %02d:%02d", min, sec)))
}

package view

import (
	"fmt"
	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// SessionStats defines the minimal interface to update session and total capture durations.
type SessionStats interface {
	SetSession(seconds int)
	SetTotal(seconds int)
}

type sessionStats struct {
	sessionLbl *LabelWidget
	totalLbl   *LabelWidget
}

// NewSessionStats creates the two labels and grids them at the provided row.
func NewSessionStats(row int) SessionStats {
	s := &sessionStats{sessionLbl: Label(Width(14)), totalLbl: Label(Width(14))}
	Grid(s.sessionLbl, Row(row), Column(0), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	Grid(s.totalLbl, Row(row), Column(1), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	s.sessionLbl.Configure(Txt("Session: 00:00"))
	s.totalLbl.Configure(Txt("Total: 00:00"))
	return s
}

func (s *sessionStats) SetSession(seconds int) {
	if s == nil || s.sessionLbl == nil {
		return
	}
	min, sec := seconds/60, seconds%60
	s.sessionLbl.Configure(Txt(fmt.Sprintf("Session: %02d:%02d", min, sec)))
}

func (s *sessionStats) SetTotal(seconds int) {
	if s == nil || s.totalLbl == nil {
		return
	}
	min, sec := seconds/60, seconds%60
	s.totalLbl.Configure(Txt(fmt.Sprintf("Total: %02d:%02d", min, sec)))
}

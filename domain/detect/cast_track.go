package detect

import "image"

// castTrackLocator rejects the strongest pre-existing colour match and waits
// for a new candidate to persist across two frames before committing to it.
type castTrackLocator struct {
	color      ColorLocator
	started    bool
	firstFrame bool
	preCast    image.Rectangle
	previous   image.Rectangle
}

func (l *castTrackLocator) BeginCast() {
	*l = castTrackLocator{started: true, firstFrame: true}
}

func (l *castTrackLocator) Locate(frame *image.RGBA) (Match, bool) {
	if !l.started {
		return Match{}, false
	}
	m, found := l.color.locate(frame, l.preCast)
	if l.firstFrame {
		l.firstFrame = false
		// shortcut: remember one pre-existing match, extend to multiple candidates if crowded-water replay warrants it.
		if found {
			l.preCast = m.Box.Add(frame.Bounds().Min)
		}
		return m, false
	}
	stable := found && image.Pt(m.X, m.Y).In(l.previous)
	l.previous = image.Rectangle{}
	if found {
		centre := image.Pt(m.X, m.Y)
		l.previous = image.Rectangle{Min: centre.Sub(image.Pt(6, 6)), Max: centre.Add(image.Pt(7, 7))}
	}
	return m, stable
}

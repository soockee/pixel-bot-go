package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// Controller receives scenario events and chooses simulated actions. Next only
// reads state, so the harness can ask again whenever anything changes. It gets
// no deadlines or future events, so a controller cannot use foreknowledge.
type Controller interface {
	OnArrive(now int64, id int)
	// OnResult reports the end of an action, including a reply after the event closed.
	OnResult(now int64, id int, ok bool)
	OnClosed(id int)
	// Next returns the next action and when it should start, or false if none is pending.
	Next(now int64) (id int, at int64, ok bool)
}

// ControllerConfig is one controller's settings. Fixed controllers use only IntervalMs.
type ControllerConfig struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	IntervalMs    int64  `json:"interval_ms,omitempty"`
	MinDelayMs    int64  `json:"min_delay_ms,omitempty"`
	MaxDelayMs    int64  `json:"max_delay_ms,omitempty"`
	BaseBackoffMs int64  `json:"base_backoff_ms,omitempty"`
	MaxBackoffMs  int64  `json:"max_backoff_ms,omitempty"`
	MaxAttempts   int    `json:"max_attempts,omitempty"`
}

// Controller kinds.
const (
	kindFixed   = "fixed"
	kindBackoff = "backoff"
)

func (c ControllerConfig) validate() error {
	switch {
	case c.Kind == kindFixed && c.IntervalMs > 0:
		return nil
	case c.Kind == kindBackoff && c.MinDelayMs >= 0 && c.MaxDelayMs >= c.MinDelayMs &&
		c.BaseBackoffMs > 0 && c.MaxBackoffMs >= c.BaseBackoffMs && c.MaxAttempts > 0 && c.MaxAttempts <= 30:
		return nil
	}
	return fmt.Errorf("config: invalid controller %q", c.Name)
}

// newController builds a controller. The backoff stream depends on the scenario
// seed and the controller name, never on the scenario's own random stream.
func newController(c ControllerConfig, seed uint64) Controller {
	if c.Kind == kindFixed {
		return &fixedInterval{interval: c.IntervalMs}
	}
	return &boundedBackoff{cfg: c, rng: rand.New(rand.NewPCG(seed, nameHash(c.Name)))}
}

// fixedInterval acts on interval ticks and retries until the event closes. A
// failed event goes to the back of the queue, so it cannot block other events.
type fixedInterval struct {
	interval int64
	pending  []int
}

func (c *fixedInterval) OnArrive(_ int64, id int) { c.pending = append(c.pending, id) }

func (c *fixedInterval) OnResult(_ int64, id int, ok bool) {
	i := slices.Index(c.pending, id)
	if i < 0 {
		return
	}
	c.pending = slices.Delete(c.pending, i, i+1)
	if !ok {
		c.pending = append(c.pending, id)
	}
}

func (c *fixedInterval) OnClosed(id int) {
	c.pending = slices.DeleteFunc(c.pending, func(p int) bool { return p == id })
}

func (c *fixedInterval) Next(now int64) (int, int64, bool) {
	if len(c.pending) == 0 {
		return 0, 0, false
	}
	return c.pending[0], (now + c.interval - 1) / c.interval * c.interval, true
}

type backoffItem struct {
	id       int
	ready    int64
	attempts int
}

// boundedBackoff waits a bounded random delay before a first action and between
// actions. Failed events retry after a capped exponential backoff, up to MaxAttempts.
type boundedBackoff struct {
	cfg           ControllerConfig
	rng           *rand.Rand
	pending       []backoffItem
	notBefore     int64
	feedbackUntil int64
	retryUntil    int64
}

func (c *boundedBackoff) delay() int64 {
	return c.cfg.MinDelayMs + c.rng.Int64N(c.cfg.MaxDelayMs-c.cfg.MinDelayMs+1)
}

func (c *boundedBackoff) OnArrive(now int64, id int) {
	c.pending = append(c.pending, backoffItem{id: id, ready: now + c.delay()})
}

func (c *boundedBackoff) OnResult(now int64, id int, ok bool) {
	c.notBefore = now + c.delay()
	i := slices.IndexFunc(c.pending, func(p backoffItem) bool { return p.id == id })
	if i < 0 {
		return
	}
	item := &c.pending[i]
	item.attempts++
	if ok || item.attempts >= c.cfg.MaxAttempts {
		c.pending = slices.Delete(c.pending, i, i+1)
		return
	}
	item.ready = now + min(c.cfg.MaxBackoffMs, c.cfg.BaseBackoffMs<<(item.attempts-1))
}

func (c *boundedBackoff) OnClosed(id int) {
	c.pending = slices.DeleteFunc(c.pending, func(p backoffItem) bool { return p.id == id })
}

func (c *boundedBackoff) Next(now int64) (int, int64, bool) {
	if len(c.pending) == 0 {
		return 0, 0, false
	}
	nextID, nextReady := c.pending[0].id, c.readyAt(c.pending[0])
	for _, p := range c.pending[1:] {
		if ready := c.readyAt(p); ready < nextReady {
			nextID, nextReady = p.id, ready
		}
	}
	return nextID, max(nextReady, c.notBefore, c.feedbackUntil, now), true
}

func (c *boundedBackoff) readyAt(p backoffItem) int64 {
	if p.attempts > 0 {
		return max(p.ready, c.retryUntil)
	}
	return p.ready
}

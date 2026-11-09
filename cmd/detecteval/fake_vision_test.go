package main

import (
	"encoding/binary"
	"image"
	"image/draw"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

const fakeSplashMS = 3960

func TestFakeGameVisionBites(t *testing.T) {
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := assets.BiteSoundPCM()
	if err != nil {
		t.Fatal(err)
	}
	names := slices.Sorted(maps.Keys(sounds))
	r := fakegame.NewRenderer(fakegame.DefaultScenario(), tmpl)
	box := r.MarkerRect()
	calm := r.Render(fakegame.PhaseBobber).SubImage(box).(*image.RGBA)
	splash := r.Render(fakegame.PhaseSplash).SubImage(box).(*image.RGBA)
	flash := image.NewRGBA(box)
	draw.Draw(flash, box, image.White, image.Point{}, draw.Src)
	type trial struct {
		name   string
		frames int
		frame  *image.RGBA
		sound  []byte
		noise  bool
		silent bool
		want   bool
	}
	trials := []trial{
		{"clean-1", 18, splash, sounds[names[0]], false, false, true},
		{"clean-2", 18, splash, sounds[names[1]], false, false, true},
		{"clean-3", 18, splash, sounds[names[2]], false, false, true},
		{"muted", 18, splash, nil, false, true, true},
		{"local-splash-and-unrelated-sound", 18, splash, nil, true, false, false},
		{"whole-crop-flash-and-other-bite", 18, flash, sounds[names[0]], false, false, false},
		{"one-frame-spray-and-other-bite", 1, splash, sounds[names[0]], false, false, false},
		{"other-bite-alone", 0, calm, sounds[names[0]], false, false, false},
	}
	for _, variant := range []struct {
		name      string
		wantFalse int
	}{
		{"splash_sound", 3}, {"splash_history", 1}, {"splash_template_sound", 2}, {"splash_history_sound", 0},
	} {
		t.Run(variant.name, func(t *testing.T) {
			var counts BiteCounts
			for _, trial := range trials {
				detector, err := detect.NewBiteDetector(variant.name, nil)
				if err != nil {
					t.Fatal(err)
				}
				decision := playFakeVisionBite(detector.(detect.SoundBiteDetector), calm, trial.frame, trial.frames, trial.sound, trial.noise, trial.silent)
				t.Logf("%s: decision_ms=%d", trial.name, decision)
				counts.Casts++
				var label *int64
				if trial.want {
					counts.BiteCasts++
					at := int64(fakeSplashMS)
					label = &at
				}
				if decision >= 0 {
					scoreBite(int64(decision), label, &counts)
				}
			}
			t.Logf("%+v", counts)
			if counts.BitesCaught != counts.BiteCasts || counts.FalseBites != variant.wantFalse {
				t.Errorf("unexpected Fake Game results: %+v", counts)
			}
		})
	}
}

func playFakeVisionBite(d detect.SoundBiteDetector, calm, effect *image.RGBA, effectFrames int, sound []byte, noise, silent bool) int {
	const packetBytes = capture.AudioRate / 100 * capture.AudioFrameBytes
	const soundMS = fakeSplashMS + 300
	var pcm [packetBytes]byte
	for ms := 0; ms < 5500; ms++ {
		if ms%10 == 0 && !silent {
			offset := (ms - soundMS) / 10 * packetBytes
			for i := 0; i < len(pcm); i += 2 {
				v := int16(5)
				if offset >= 0 && offset+i+2 <= len(sound) {
					v += int16(binary.LittleEndian.Uint16(sound[offset+i:])) / 5
				}
				if noise && ms >= soundMS && ms < soundMS+150 {
					v += 300 - int16(i/2%20)*30
				}
				binary.LittleEndian.PutUint16(pcm[i:], uint16(v))
			}
			if d.Hear(pcm[:], time.UnixMilli(int64(ms))) {
				return ms + 10
			}
		}
		if ms%33 == 0 {
			frame := calm
			if ms >= fakeSplashMS && ms < fakeSplashMS+effectFrames*33 {
				frame = effect
			}
			if d.Feed(frame, time.UnixMilli(int64(ms))) {
				return ms
			}
		}
	}
	return -1
}

func TestFakeGameVisionLocators(t *testing.T) {
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	scene := fakegame.DefaultScenario()
	ours := fakegame.NewRenderer(scene, tmpl)
	scene.BobberX -= 110
	other := fakegame.NewRenderer(scene, tmpl)
	empty := ours.RenderUnmarked(fakegame.PhaseEmpty)
	landed := ours.RenderUnmarked(fakegame.PhaseBobber)
	distractor := other.RenderUnmarked(fakegame.PhaseBobber)
	copyWithOther := image.NewRGBA(landed.Bounds())
	draw.Draw(copyWithOther, landed.Bounds(), landed, image.Point{}, draw.Src)
	draw.Draw(copyWithOther, other.BobberRect(), distractor, other.BobberRect().Min, draw.Src)
	for _, name := range []string{"color", "ncc", "cast_track"} {
		t.Run(name, func(t *testing.T) {
			l, err := detect.NewLocator(name, *config.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			for _, trial := range []struct {
				name          string
				before, after *image.RGBA
			}{
				{"clean", empty, landed},
				{"pre-existing-bobber", distractor, copyWithOther},
			} {
				if c, ok := l.(detect.CastLocator); ok {
					c.BeginCast()
				}
				lock, right := -1, false
				for i, frame := range []*image.RGBA{trial.before, trial.before, trial.after, trial.after, trial.after} {
					if m, found := l.Locate(frame); found {
						lock, right = i*66, image.Pt(m.X, m.Y).In(ours.BobberRect())
						break
					}
				}
				t.Logf("%s: lock_ms=%d correct=%t", trial.name, lock, right)
				if name == "cast_track" && !right {
					t.Errorf("cast tracker failed to find the new bobber in %s", trial.name)
				}
				if name == "cast_track" && right {
					for i, frame := range []*image.RGBA{trial.before, trial.after, trial.after} {
						m, found := l.Locate(frame)
						if found != (i == 2) || (found && !image.Pt(m.X, m.Y).In(ours.BobberRect())) {
							t.Errorf("reacquisition frame %d: match=%+v found=%t", i, m, found)
						}
					}
					t.Log("reacquired on the second observation after one missing frame")
				}
			}
		})
	}
}

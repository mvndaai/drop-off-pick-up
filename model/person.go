package model

import "math/rand"

// Person represents an individual being dropped off or picked up.
type Person struct {
	ID   string
	Name string
	// DropOffSeconds is the baseline drop-off handling time for this person.
	DropOffSeconds float32
	// DropOffVarianceSeconds is a random +/- variance applied to DropOffSeconds.
	DropOffVarianceSeconds float32
	// WalkSeconds is the baseline walking time toward or from the crosswalk area.
	WalkSeconds float32
}

// EffectiveDropOffSeconds returns a randomized per-person drop-off duration.
func (p *Person) EffectiveDropOffSeconds() float32 {
	base := p.DropOffSeconds
	if base <= 0 {
		base = 2
	}
	if p.DropOffVarianceSeconds <= 0 {
		return base
	}
	jitter := (rand.Float32()*2 - 1) * p.DropOffVarianceSeconds //nolint:gosec
	d := base + jitter
	if d < 0.5 {
		return 0.5
	}
	return d
}

package model

// Mode represents the operational mode of the simulation.
type Mode int

const (
	ModeDropOff Mode = iota
	ModePickUp
)

func (m Mode) String() string {
	if m == ModeDropOff {
		return "Drop-Off"
	}
	return "Pick-Up"
}

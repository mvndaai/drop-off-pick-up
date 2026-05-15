package model

import "image/color"

// DropOffStyle describes how passengers exit the vehicle during drop-off.
type DropOffStyle int

const (
	// DropOffSelf – passenger exits on their own (~2 s).
	DropOffSelf DropOffStyle = iota
	// DropOffAssisted – a staff member assists the passenger (~5 s).
	DropOffAssisted
)

// Duration returns the simulated seconds required for this drop-off style.
func (d DropOffStyle) Duration() float32 {
	if d == DropOffAssisted {
		return 5.0
	}
	return 2.0
}

func (d DropOffStyle) String() string {
	if d == DropOffAssisted {
		return "Assisted"
	}
	return "Self"
}

// Vehicle represents a car or bus travelling through the facility.
type Vehicle struct {
	ID           string
	LicensePlate string
	Passengers   []*Person
	Mode         Mode
	DropStyle    DropOffStyle
	Color        color.RGBA
}

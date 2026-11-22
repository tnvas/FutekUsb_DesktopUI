// Package physics converts force values between the units supported by the
// application. Newton is the canonical internal unit.
package physics

import "fmt"

// Supported unit identifiers.
const (
	N   = "N"   // newton
	LBS = "lbs" // pound-force
	KG  = "kg"  // kilogram-force
	G   = "g"   // gram-force
	OZ  = "oz"  // ounce-force
)

// Units lists the selectable display units.
var Units = []string{N, KG, G, LBS, OZ}

// Newtons per unit of force.
const (
	newtonsPerLbf = 4.4482216152605
	newtonsPerOzf = newtonsPerLbf / 16
	newtonsPerKgf = 9.80665
	newtonsPerGf  = newtonsPerKgf / 1000
)

// Convert converts a value between two supported units.
func Convert(from, to string, val float64) (float64, error) {
	if from == to {
		return val, nil
	}
	n, err := ToNewtons(from, val)
	if err != nil {
		return 0, err
	}
	return FromNewtons(to, n)
}

// ToNewtons converts a value in the given unit to Newtons.
func ToNewtons(from string, v float64) (float64, error) {
	switch normalize(from) {
	case N:
		return v, nil
	case LBS:
		return v * newtonsPerLbf, nil
	case OZ:
		return v * newtonsPerOzf, nil
	case KG:
		return v * newtonsPerKgf, nil
	case G:
		return v * newtonsPerGf, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", from)
	}
}

// FromNewtons converts a value in Newtons to the given unit.
func FromNewtons(to string, n float64) (float64, error) {
	switch normalize(to) {
	case N:
		return n, nil
	case LBS:
		return n / newtonsPerLbf, nil
	case OZ:
		return n / newtonsPerOzf, nil
	case KG:
		return n / newtonsPerKgf, nil
	case G:
		return n / newtonsPerGf, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", to)
	}
}

// normalize maps legacy unit spellings from old databases ("os" was a typo
// for ounce in earlier releases) onto current identifiers.
func normalize(u string) string {
	if u == "os" {
		return OZ
	}
	return u
}

// echo provides ultrasonic echolocation to measure a tank's liquid level
package echo

import (
	"errors"
	"machine"
	"math"
	"strconv"
	"strings"

	"github.com/eyelight/trigger"
	"tinygo.org/x/drivers/hcsr04"
)

const (
	ERR_CALIBRATION_REQUIRED     = "tank calibration required"
	ERR_FULL_CALIBRATION_FAILED  = "calibration failed - 'full' would become deeper than 'empty'"
	ERR_EMPTY_CALIBRATION_FAILED = "calibration failed - 'empty' would become shallower than 'full'"
)

const pi float64 = 3.14159

type TankShape int

const (
	Cylinder TankShape = iota
	Cuboid
	Sphere
)

// String returns the string value of a TankShape
func (t TankShape) String() string {
	switch t {
	case Cuboid:
		return "Cuboid"
	case Sphere:
		return "Sphere"
	default: // Cylinder
		return "Cylinder"
	}
}

type LengthUnit int // in which tank dimensions are specified in a TankConfig

const (
	Centimeter LengthUnit = iota
	Millimeter
	Meter
	Inch
	Foot
)

// String returns the string value of a LengthUnit
func (l LengthUnit) String() string {
	switch l {
	case Millimeter:
		return "mm"
	case Meter:
		return "m"
	case Inch:
		return "in"
	case Foot:
		return "ft"
	default: // Centimeter
		return "cm"
	}
}

// conv returns a conversion factor from a LengthUnit into centimeters which echo uses internally
func (l LengthUnit) conv() float64 {
	switch l {
	case Millimeter:
		return 0.1
	case Meter:
		return 100.0
	case Inch:
		return 2.54
	case Foot:
		return 30.48
	default: // Centimeter
		return 1.0
	}
}

type VolumeUnit int // in which volumetric value is returned to callers of Read()

const (
	Milliliter VolumeUnit = iota // aka cubic centimeter
	Liter
	Ounce
	Pint
	Quart
	Gallon
)

func (v VolumeUnit) String() string {
	switch v {
	case Liter:
		return "L"
	case Ounce:
		return "oz"
	case Pint:
		return "pt"
	case Quart:
		return "qt"
	case Gallon:
		return "gal"
	default: // Milliliter
		return "mL"
	}
}

// conv returns the conversion factor from the default of mL to a VolumeUnit
func (v VolumeUnit) conv() float64 {
	switch v {
	case Liter:
		return 0.001
	case Ounce:
		return 0.0338
	case Pint:
		return 0.002113
	case Quart:
		return 0.001057
	case Gallon:
		return 0.000264172
	default:
		return 1.0
	}
}

type tank struct {
	d         *hcsr04.Device
	name      string
	shape     TankShape
	lu        LengthUnit // length unit preferred by the consumer
	vu        VolumeUnit // volume unit preferred by the consumer
	r         float64    // radius in centimeters (use for spheroid & cylinder tanks)
	h         float64    // height in centimeters (use for spheroid & cylinder tanks)
	s1        float64    // side1 in centimeters (use for cuboid tanks)
	s2        float64    // side2 in centimeters (used for cuboid tanks)
	fullDist  int32      // calibratable distance representing 'full'
	emptyDist int32      // calibratable distance representing 'empty'
}

type TankConf struct {
	name       string     // a nickname for the tank
	shape      TankShape  // the shape of the tank dictates the distance-to-volume conversion
	lengthUnit LengthUnit // preferred units of r, h, s1, and s2, internally converted to centimeter
	volumeUnit VolumeUnit // preferred units to which volume readings will be converted
	r          uint32     // number in LengthUnit representing the radius of a cylinder or spherical tank
	h          uint32     // number in LengthUnit representing the height of a tank
	s1         uint32     // number in LengthUnit representing side1 of a cuboid tank
	s2         uint32     // number in LengthUnit representing side2 of a cuboid tank
}

type Tank interface {
	Configure(TankConf)              // sets up a tank for calibration
	Calibrate(bool) error            // false calibrates empty / true calibrates full
	Execute(trigger.Trigger)         // stub to satisfy the trigger.Triggerable interface
	Name() string                    // returns the tank's name to satisfy the trigger.Triggerable interface
	Read() (float64, float64, error) // returns percentage full, contained volume as VolumeUnit units
	String() string                  // describes the tank with its relevant information
}

// New returns an unconfigured Tank using the passed-in pins
func New(trigger, echo machine.Pin) Tank {
	dev := hcsr04.New(trigger, echo)
	return &tank{
		d: &dev,
	}
}

// Configure sets up a Tank for use, defaulting to a Cylinder of size 0 Millimeters & reporting in Milliliters
func (t *tank) Configure(tc TankConf) {
	if tc.name == "" {
		t.name = "MyTank"
	} else {
		t.name = tc.name
	}
	t.shape = tc.shape
	t.lu = tc.lengthUnit
	t.vu = tc.volumeUnit
	t.r = t.cm(tc.r)   // convert to centimeters for internal use
	t.h = t.cm(tc.h)   // convert to centimeters for internal use
	t.s1 = t.cm(tc.s1) // convert to centimeters for internal use
	t.s2 = t.cm(tc.s2) // convert to centimeters for internal use
}

// Calibrate takes a bool indicating which distance to calibrate,
// reads the current distance (mm), checks for sanity,
// and updates the tank's fullDist or emptyDist;
// TODO: it then notifies the eeprom of the change
func (t *tank) Calibrate(full bool) error {
	c := t.d.ReadDistance()
	if full {
		// ensure fullDist calibration has a reading below emptyDist
		if c >= t.emptyDist {
			return errors.New(ERR_FULL_CALIBRATION_FAILED)
		}
		// set fullDist to the calibration reading
		t.fullDist = c
	} else {
		// ensure emptyDist calibration has a reading above fullDist
		if c <= t.fullDist {
			return errors.New(ERR_EMPTY_CALIBRATION_FAILED)
		}
		// set emptyDist to the calibration reading
		t.emptyDist = c
	}
	// TODO: notify eeprom of change
	return nil
}

// Execute performs an action sent via mqtt for which the tank is the target
func (t *tank) Execute(trigger.Trigger) {
	// TODO: implement actions
	// TODO: notify eeprom if action needs persistence
}

// Name returns the tank's name to comply with trigger.Triggerable
func (t *tank) Name() string {
	return t.name
}

// Read returns a percentage full and volumetric measurement in the preferred VolumeUnits,
// a string of which is also returned as the error value.
// A return of -420.69 indicates an uncalibrated tank; the user should be prompted to calibrate
func (t *tank) Read() (float64, float64, error) {
	if t.emptyDist == 0 || t.fullDist == 0 {
		return -420.69, -420.69, errors.New(ERR_CALIBRATION_REQUIRED)
	}
	r := t.d.ReadDistance()
	pct := float64((t.emptyDist - r) / (t.emptyDist - t.fullDist))
	ml := t.ml(r)
	return pct, ml, errors.New(t.vu.String())
}

// String returns a string with relevant information about a tank
func (t *tank) String() string {
	ss := strings.Builder{}
	ss.Grow(128)
	ss.WriteString("Tank Shape: ")
	ss.WriteString(t.shape.String())
	switch t.shape {
	case Cuboid:
		ss.WriteString(" Length: ")
		ss.WriteString(strconv.FormatFloat(t.s1/t.lu.conv(), 'f', 2, 64))
		ss.WriteString(t.lu.String())
		ss.WriteString(" Width: ")
		ss.WriteString(strconv.FormatFloat(t.s2/t.lu.conv(), 'f', 2, 64))
		ss.WriteString(t.lu.String())
	case Cylinder:
		ss.WriteString(" Height: ")
		ss.WriteString(strconv.FormatFloat(t.h/t.lu.conv(), 'f', 2, 64))
		ss.WriteString(t.lu.String())
		ss.WriteString(" Radius: ")
		ss.WriteString(strconv.FormatFloat(t.r/t.lu.conv(), 'f', 2, 64))
		ss.WriteString(t.lu.String())
	case Sphere:
		ss.WriteString(" Radius: ")
		ss.WriteString(strconv.FormatFloat(t.r/t.lu.conv(), 'f', 2, 64))
		ss.WriteString(t.lu.String())
	}
	ss.WriteString(" Calibrated Capacity: ")
	ss.WriteString(strconv.FormatFloat(t.ml(t.fullDist), 'f', 2, 64))
	ss.WriteString(t.vu.String())
	return ss.String()
}

// ml returns a milliliter value indicating tank volume from a passed-in distance reading
func (t *tank) ml(mm int32) float64 {
	cm := float64((t.emptyDist - mm) / 10)
	var ml float64
	switch t.shape {
	case Cylinder:
		// cubic centimeter (mL) volume of a cylinder = pi * r^2 * h
		ml = math.Pow(t.r, 2.0) * pi * cm
	case Sphere:
		// spherical cap in cubic cm (mL) = (1/6)pi * h * (3r^2 + h^2)
		ml = (1 / 6) * pi * cm * (3*math.Pow(t.r, 2) + math.Pow(cm, 2))
	case Cuboid:
		// cuboid volume in cubic cm (mL) = l * w * h
		ml = t.s1 * t.s2 * cm
	}
	return ml
}

// cm converts and returns in centimeters the passed in value converted from the tank's LengthUnit
func (t *tank) cm(input uint32) float64 {
	return t.lu.conv() * float64(input)
}

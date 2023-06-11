# echo
echo is a package for defining liquid tanks of various shapes and taking volume readings via echolocation using an ultrasonic distance sensor on microcontroller firmware code written in [TinyGo](https://tinygo.org). 

By installing an ultrasonic rangefinder at the top of a liquid tank, the distance to the water table can be obtained by virtue of the difference in density of the air versus the water. The sensor records the time of flight of the sound waves from when they are emitted from one transducer to the time an echo off the surface of the water hits a second transducer. 

After giving `echo` the shape and dimensions of your tank, and calibrating the distances for 'full' and 'empty', `echo` will determine the volume held by the tank when you call the `Read()` method.

Because we are converting distance measurements to volumetric, the simplest conversion is from centimeter to cubic centimeter, aka the milliliter (mL). Internally, `echo` uses the centimeter for its calculations, and will convert all centimeter and milliliter readings to your preferred units. When setting up your tank dimensions, ensure the tank dimensions are denominated in the units you're providing for LengthUnit.

### Installation

It is assumed that you have an environment with [Golang](https://go.dev), [TinyGo](https://github.com/tinygo-org/tinygo), and TinyGo's [drivers](https://github.com/tinygo-org/drivers) repository, which contains a driver for the HC-SR04 ultrasonic rangefinder ([datasheet](https://cdn.sparkfun.com/datasheets/Sensors/Proximity/HCSR04.pdf)) used by `echo`. 

#### Install dependencies
~~Trigger is a way to send messages around within your MCU. Your MQTT listener can translate incoming messages into a `Trigger` and sent to `Triggerable` entities, which `echo` is designed to be.~~ The triggerable interface is stubbed but not yet implemented. But you may get some complaints unless you install it.

```bash
go get github.com/eyelight/trigger
```
The `trigger.Triggerable` interface is as follows:
```go
// github.com/eyelight/trigger/trigger.go
type Triggerable interface {
    Name() string
    Execute(Trigger)
}
```
If you add a file of your own in package `echo` that implements `Execute(trigger.Trigger)`, you can add it to an ecosystem that utilizes it. I may split this out into a different interface (eg TriggerableTank) so there is no dependency.

#### Install echo

```bash
go get github.com/eyelight/echo
```

### Usage

Import the package:
```go
import "github.com/eyelight/echo"
```

Set up your tank. Maybe it's a fermenter because you're brewing kombucha or beer:
```go
fermenter := echo.New(machine.D2, machine.D3)
fermenter.Configure(echo.TankConfig{
    name: "MyKombuchaFermenter", // pass this to a trigger.Dispatcher
    shape: echo.Cylinder, // Cylinder, Cuboid, or Sphere
    lengthUnit: echo.Inch, // the length unit you're using for tank dimensions
    volumeUnit: echo.Gallon, // the volume unit you're using for readings
    r: 36, // integer radius of the tank in `lengthUnit` units (eg, echo.Inch)
    h: 60, // integer height of the tank in `lengthUnit` units (eg, echo.Inch)
})
println(fermenter.String()) // report the tank's parameters
```

#### Calibration
To properly make readings, `echo` needs to know the distance for 'full' and for 'empty'. You can call `Calibrate(bool)` passing `false` to update the calibration distance for 'empty', or `true` for 'full." 

Ensure the tank is empty (or, for a non-spherical tank, the level you'd like to calibrate as empty):
```go
fermenter.Calibrate(false)
```

Ensure the tank is full (or, for a non-spherical tank, the level you'd like to calibrate as full):
```go
fermenter.Calibrate(true)
```

#### Getting Readings
Call `Read()` to perform an echolocation and cause `echo` to convert the distance to the tank's fill percentage, as well as the absolute volume in your preferred volume units. 

Because you can set the calibration in order to provide a safe margins for top/bottom distance, the percentage and volume could potentially return negative. This could also be the case if the tank is uncalibrated.

An error is also returned – **which is never `nil`**. The error will usually be an error-wrapped string containing the volumetric unit of the returned volume, to mitigate against unit mismatches. Or, if the tank needs calibration, the returned error will be an actual error indicating that need. In that case, the returned percentage and volume will also be absurd.

Instead of checking for a nil error here, check if the returned percentage/volume values are `-420.69`.

```go
pct, vol, units := fermenter.Read()
if pct, vol == -420.69, -420.69 {
    println(units.Error()) // "tank calibration required"
}
printf("%f %s \n", vol, units.Error()) // "27.30 gal"
```

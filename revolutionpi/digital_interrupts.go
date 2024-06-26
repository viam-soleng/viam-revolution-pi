//go:build linux

// Package revolutionpi implements the Revolution Pi board GPIO pins.
package revolutionpi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	inputModeOffset = 88
)

// counterPin is the struct used for configuring an interrupt or encoder.
// encoders and digital interrupts are configured the same way in the revolution pi.
// the encoder & digital interrupt interface cannot be satisfied by the same struct due
// to both interfaces requiring different Name() methods.
// Go does not allow for overloads, so instead the revolutionPiEncoder and diWrapper structs
// are used to implement their respective interfaces.
type counterPin struct {
	pinName          string // Variable name
	address          uint16 // address of the byte in the process image
	length           uint16 // length of the variable in bits. Possible values are 1, 8, 16 and 32
	bitPosition      uint8  // 0-7 bit position, >= 8 whole byte, only used if the digital input pin is given
	controlChip      *gpioChip
	outputOffset     uint16
	inputOffset      uint16
	enabled          bool
	interruptAddress uint16
}

// diWrapper wraps a digital interrupt pin with the DigitalInterrupt interface.
type diWrapper struct {
	pin *counterPin
}

func initializeDigitalInterrupt(pin SPIVariable, g *gpioChip, isEncoder bool) (*counterPin, error) {
	di := counterPin{
		pinName: str32(pin.strVarName), address: pin.i16uAddress,
		length: pin.i16uLength, bitPosition: pin.i8uBit, controlChip: g,
	}
	g.logger.Debugf("setting up digital interrupt pin: %v", di)
	dio, err := findDevice(di.address, g.dioDevices)
	if err != nil {
		return &counterPin{}, err
	}
	// store the input & output offsets of the board for quick reference
	di.outputOffset = dio.i16uOutputOffset
	di.inputOffset = dio.i16uInputOffset

	var addressInputMode uint16

	// read from the input mode byte to determine if the pin is configured for counter/interrupt mode
	// determine which address to check for the input mode based on which pin was given in the request
	switch {
	case di.isInputCounter():
		addressInputMode = (di.address - di.inputOffset - inputWordToCounterOffset) >> 2

		// record the address for the interrupt
		di.interruptAddress = di.address
	case di.isDigitalInput():
		addressInputMode = uint16(di.bitPosition)
		if di.address > di.inputOffset { // This is the second set of input pins, so move the offset over
			addressInputMode += 8
		}
		di.interruptAddress = di.inputOffset + inputWordToCounterOffset + addressInputMode*4
	default:
		return &counterPin{}, errors.New("pin is not a digital input pin")
	}

	b := make([]byte, 1)
	// read from the input mode addresses to see if the pin is configured for interrupts
	n, err := di.controlChip.fileHandle.ReadAt(b, int64(di.inputOffset+inputModeOffset+addressInputMode))
	if err != nil {
		return &counterPin{}, err
	}
	if n != 1 {
		return &counterPin{}, errors.New("unable to read digital input pin configuration")
	}
	di.controlChip.logger.Debugf("Current Pin configuration: %#d", b)

	// check if the pin is configured as a counter
	// b[0] == 0 means the interrupt is disabled, b[0] == 3 means the pin is configured for encoder mode
	if b[0] == 0 || (b[0] == 3 && !isEncoder) {
		return &counterPin{}, fmt.Errorf("pin %s is not configured as a counter", di.pinName)
	} else if b[0] != 3 && isEncoder {
		return &counterPin{}, fmt.Errorf("pin %s is not configured as an encoder", di.pinName)
	}

	di.enabled = true

	return &di, nil
}

func (di *diWrapper) Value(ctx context.Context, extra map[string]interface{}) (int64, error) {
	val, err := di.pin.Value()
	if err != nil {
		return 0, err
	}
	return int64(val), nil
}

// Note: The revolution pi only supports uint32 counters, while the Value API expects int64.
func (di *counterPin) Value() (uint32, error) {
	if !di.enabled {
		return 0, fmt.Errorf("cannot get digital interrupt value, pin %s is not configured as an interrupt", di.pinName)
	}
	di.controlChip.logger.Debugf("Reading from %d, length: 4 byte(s)", di.interruptAddress)
	b := make([]byte, 4)
	n, err := di.controlChip.fileHandle.ReadAt(b, int64(di.interruptAddress))
	if err != nil {
		return 0, err
	}
	di.controlChip.logger.Debugf("Read %#v bytes", b)
	if n != 4 {
		return 0, fmt.Errorf("expected 4 bytes, got %#v", b)
	}
	val := binary.LittleEndian.Uint32(b)
	return val, nil
}

func (di *diWrapper) Name() string {
	return di.pin.pinName
}

// addresses at 6 to 70 + inputOffset.
func (di *counterPin) isInputCounter() bool {
	return di.address >= di.inputOffset+inputWordToCounterOffset && di.address < di.outputOffset
}

// addresses at 0 and 1 + inputOffset.
func (di *counterPin) isDigitalInput() bool {
	return di.address == di.inputOffset || di.address == di.inputOffset+1
}

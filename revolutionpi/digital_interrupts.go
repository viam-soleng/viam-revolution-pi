//go:build linux

// Package genericlinux is for Linux boards, and this particular file is for digital interrupt pins
// using the ioctl interface, indirectly by way of mkch's gpio package.
package revolutionpi

import (
	"context"
	"sync"

	"github.com/mkch/gpio"
	"github.com/pkg/errors"
	"go.viam.com/utils"

	"go.uber.org/multierr"
	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/board/pinwrappers"
)

type digitalInterrupt struct {
	boardWorkers *sync.WaitGroup
	interrupt    pinwrappers.ReconfigurableDigitalInterrupt
	line         *gpio.LineWithEvent
	cancelCtx    context.Context
	cancelFunc   func()
	config       *board.DigitalInterruptConfig

	Name         string // Variable name
	Address      uint16 // Address of the byte in the process image
	Length       uint16 // length of the variable in bits. Possible values are 1, 8, 16 and 32
	ControlChip  *gpioChip
	outputOffset uint16
	inputOffset  uint16
}

// func (b *Board) createDigitalInterrupt(
// 	ctx context.Context,
// 	config board.DigitalInterruptConfig,
// 	gpioMappings map[string]GPIOBoardMapping,
// 	// If we are reconfiguring a board, we might already have channels subscribed and listening for
// 	// updates from an old interrupt that we're creating on a new pin. In that case, reuse the part
// 	// that holds the callbacks.
// 	oldCallbackHolder pinwrappers.ReconfigurableDigitalInterrupt,
// ) (*digitalInterrupt, error) {
// 	mapping, ok := gpioMappings[config.Pin]
// 	if !ok {
// 		return nil, errors.Errorf("unknown interrupt pin %s", config.Pin)
// 	}

// 	chip, err := gpio.OpenChip(mapping.GPIOChipDev)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer utils.UncheckedErrorFunc(chip.Close)

// 	line, err := chip.OpenLineWithEvents(
// 		uint32(mapping.GPIO), gpio.Input, gpio.BothEdges, "viam-interrupt")
// 	if err != nil {
// 		return nil, err
// 	}

// 	var interrupt pinwrappers.ReconfigurableDigitalInterrupt
// 	if oldCallbackHolder == nil {
// 		interrupt, err = pinwrappers.CreateDigitalInterrupt(config)
// 		if err != nil {
// 			return nil, multierr.Combine(err, line.Close())
// 		}
// 	} else {
// 		interrupt = oldCallbackHolder
// 		if err := interrupt.Reconfigure(config); err != nil {
// 			return nil, err // Should never have errors, but this makes the linter happy
// 		}
// 	}

//		cancelCtx, cancelFunc := context.WithCancel(ctx)
//		result := digitalInterrupt{
//			boardWorkers: &b.activeBackgroundWorkers,
//			interrupt:    interrupt,
//			line:         line,
//			cancelCtx:    cancelCtx,
//			cancelFunc:   cancelFunc,
//			config:       &config,
//		}
//		result.startMonitor()
//		return &result, nil
//	}
func (di *digitalInterrupt) initialize() error {
	di.ControlChip.logger.Info("yo dev path: ", di.ControlChip.dev)
	chip, err := gpio.OpenChip("piControl0")
	if err != nil {
		return err
	}
	defer utils.UncheckedErrorFunc(chip.Close)

	di.ControlChip.logger.Info("yo chip: ", chip)
	line, err := chip.OpenLineWithEvents(
		uint32(di.Address), gpio.Input, gpio.BothEdges, "viam-interrupt")
	if err != nil {
		return err
	}

	cfg := board.DigitalInterruptConfig{Name: di.Name, Pin: di.Name}
	interrupt, err := pinwrappers.CreateDigitalInterrupt(cfg)
	if err != nil {
		return multierr.Combine(err, line.Close())
	}
	di.line = line
	di.interrupt = interrupt
	di.config = &cfg

	// result := digitalInterrupt{
	// 	boardWorkers: &b.activeBackgroundWorkers,
	// 	interrupt:    interrupt,
	// 	line:         line,
	// 	cancelCtx:    cancelCtx,
	// 	cancelFunc:   cancelFunc,
	// 	config:       &config,
	// }
	return nil
}

func (di *digitalInterrupt) startMonitor() {
	di.boardWorkers.Add(1)
	utils.ManagedGo(func() {
		for {
			select {
			case <-di.cancelCtx.Done():
				return
			case event := <-di.line.Events():
				utils.UncheckedError(di.interrupt.Tick(
					di.cancelCtx, event.RisingEdge, uint64(event.Time.UnixNano())))
			}
		}
	}, di.boardWorkers.Done)
}

func (di *digitalInterrupt) Close() error {
	// We shut down the background goroutine that monitors this interrupt, but don't need to wait
	// for it to finish shutting down because it doesn't use anything in the line itself (just a
	// channel of events that the line generates). It will shut down sometime soon, and if that's
	// after the line is closed, that's fine.
	di.cancelFunc()
	return di.line.Close()
}

// struct implements board.GPIOPin to support reading current state of digital interrupt pins as GPIO inputs.
type gpioInterruptWrapperPin struct {
	interrupt digitalInterrupt
}

func (gp gpioInterruptWrapperPin) Set(
	ctx context.Context, isHigh bool, extra map[string]interface{},
) error {
	return errors.New("cannot set value of a digital interrupt pin")
}

func (gp gpioInterruptWrapperPin) Get(ctx context.Context, extra map[string]interface{}) (result bool, err error) {
	value, err := gp.interrupt.line.Value()
	if err != nil {
		return false, err
	}

	// We'd expect value to be either 0 or 1, but any non-zero value should be considered high.
	return (value != 0), nil
}

func (gp gpioInterruptWrapperPin) PWM(ctx context.Context, extra map[string]interface{}) (float64, error) {
	return 0, errors.New("cannot get PWM of a digital interrupt pin")
}

func (gp gpioInterruptWrapperPin) SetPWM(
	ctx context.Context, dutyCyclePct float64, extra map[string]interface{},
) error {
	return errors.New("cannot set PWM of a digital interrupt pin")
}

func (gp gpioInterruptWrapperPin) PWMFreq(
	ctx context.Context, extra map[string]interface{},
) (uint, error) {
	return 0, errors.New("cannot get PWM freq of a digital interrupt pin")
}

func (gp gpioInterruptWrapperPin) SetPWMFreq(
	ctx context.Context, freqHz uint, extra map[string]interface{},
) error {
	return errors.New("cannot set PWM freq of a digital interrupt pin")
}
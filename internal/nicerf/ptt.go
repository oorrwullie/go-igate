package nicerf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNotArmed  = errors.New("nicerf: PTT controller not armed")
	ErrPTTFault  = errors.New("nicerf: PTT controller faulted")
	ErrPTTClosed = errors.New("nicerf: PTT controller closed")
)

// PTT controls the logical transmit enable. For the SA818S and SA868S,
// enabled=true means PTT driven low; false means PTT released (receive).
// An implementation must honor ctx even if its underlying hardware stalls.
type PTT interface {
	SetTransmit(ctx context.Context, enabled bool) error
}

// FramePlayer must return only after audio output is complete, not merely
// after samples have been queued. It must honor cancellation promptly.
type FramePlayer interface {
	PlayFrame(ctx context.Context, frame string) error
}

// PTTController serializes complete frame transmissions. It does not configure
// a UART, drive real GPIO, control antenna relays, or protect an SDR receiver.
// Closing cancels active work before waiting for it to release the gate.
type PTTController struct {
	gate     PTT
	audio    FramePlayer
	stopCtx  context.Context
	cancel   context.CancelFunc
	token    chan struct{}
	stopOnce sync.Once
	stopped  chan struct{}
	result   error // written only by stopOnce goroutine before stopped closes
	armed    bool  // only accessed while holding token
	fault    error // only accessed while holding token
}

const releaseTimeout = 500 * time.Millisecond

func NewPTTController(gate PTT, audio FramePlayer) (*PTTController, error) {
	if gate == nil || audio == nil {
		return nil, errors.New("nicerf: PTT gate and frame player are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &PTTController{
		gate: gate, audio: audio, stopCtx: ctx, cancel: cancel,
		token: make(chan struct{}, 1), stopped: make(chan struct{}),
	}
	c.token <- struct{}{}
	return c, nil
}

// Arm confirms the receive/off state before allowing transmissions.
// It never asserts PTT and is intentionally separate from construction.
func (c *PTTController) Arm(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nicerf: nil context")
	}
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.releaseToken()
	if c.fault != nil {
		return fmt.Errorf("%w: %v", ErrPTTFault, c.fault)
	}
	if c.armed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.gate.SetTransmit(ctx, false); err != nil {
		c.fault = fmt.Errorf("initial receive state: %w", err)
		return fmt.Errorf("%w: %v", ErrPTTFault, c.fault)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.stopCtx.Err(); err != nil {
		return ErrPTTClosed
	}
	c.armed = true
	return nil
}

// Transmit asserts PTT, waits for audio to finish, and then releases PTT.
// A failed assertion still triggers release, because the gate may have moved.
// Any release failure permanently faults the controller.
func (c *PTTController) Transmit(ctx context.Context, frame string) (retErr error) {
	if ctx == nil {
		return errors.New("nicerf: nil context")
	}
	if frame == "" {
		return errors.New("nicerf: empty frame")
	}
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.releaseToken()
	if c.fault != nil {
		return fmt.Errorf("%w: %v", ErrPTTFault, c.fault)
	}
	if !c.armed {
		return ErrNotArmed
	}
	combined, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.stopCtx, cancel)
	defer func() { stop(); cancel() }()
	if err := combined.Err(); err != nil {
		return err
	}
	// Release on every attempted key, even if SetTransmit(true) fails.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), releaseTimeout)
		defer cleanupCancel()
		if err := c.gate.SetTransmit(cleanupCtx, false); err != nil {
			c.fault = fmt.Errorf("release PTT: %w", err)
			retErr = errors.Join(retErr, fmt.Errorf("%w: %v", ErrPTTFault, c.fault))
		}
	}()
	if err := c.gate.SetTransmit(combined, true); err != nil {
		return fmt.Errorf("assert PTT: %w", err)
	}
	if err := combined.Err(); err != nil {
		return err
	}
	if err := c.audio.PlayFrame(combined, frame); err != nil {
		return fmt.Errorf("play frame: %w", err)
	}
	if err := combined.Err(); err != nil {
		return err
	}
	return nil
}

func (c *PTTController) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.stopCtx.Done():
		return ErrPTTClosed
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stopCtx.Done():
		return ErrPTTClosed
	case <-c.token:
	}
	// A token and cancellation may become ready simultaneously.
	if err := ctx.Err(); err != nil {
		c.releaseToken()
		return err
	}
	if c.stopCtx.Err() != nil {
		c.releaseToken()
		return ErrPTTClosed
	}
	return nil
}

func (c *PTTController) releaseToken() { c.token <- struct{}{} }

// Close cancels active playback and attempts a final receive-state command.
// If the GPIO driver cannot release PTT, Close reports the failure; external
// fail-safe hardware is required to handle that condition and process crashes.
func (c *PTTController) Close() error {
	c.stopOnce.Do(func() {
		c.cancel()
		<-c.token
		cleanupCtx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
		c.result = c.gate.SetTransmit(cleanupCtx, false)
		cancel()
		c.armed = false
		c.releaseToken()
		close(c.stopped)
	})
	<-c.stopped
	return c.result
}

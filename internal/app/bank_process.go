package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type ownedBank struct {
	cmd     *exec.Cmd
	done    chan struct{}
	exitErr error
}

func startOwnedBank(ctx context.Context, path string, args ...string) (*ownedBank, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The run owns cleanup. Input cancellation must leave the same application
	// alive long enough for its final read-only reconciliation capture.
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	b := &ownedBank{cmd: cmd, done: make(chan struct{})}
	go func() { b.exitErr = cmd.Wait(); close(b.done) }()
	return b, nil
}
func (b *ownedBank) admissionError(err error) error {
	if b == nil {
		return err
	}
	select {
	case <-b.done:
		return errors.Join(err, fmt.Errorf("bank exited before attachment: %v", b.exitErr))
	default:
		return err
	}
}
func (b *ownedBank) stop() error {
	if b == nil {
		return nil
	}
	err := b.cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-b.done:
		return err
	case <-timer.C:
		return errors.Join(err, errors.New("owned bank did not exit within two-second cleanup deadline"))
	}
}

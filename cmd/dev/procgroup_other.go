//go:build !unix

package main

import "os/exec"

// ownProcessGroup does nothing here: exec's cancel kills the direct child, and
// a plan stopped by a failure may leave that child's own children running.
func ownProcessGroup(*exec.Cmd) {}

// stopProcessGroup does nothing here, for the same reason.
func stopProcessGroup(*exec.Cmd) {}

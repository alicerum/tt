//go:build !darwin

package main

import "os"

// ttyPending: only macOS drains the terminal queue; elsewhere a paste's
// extra lines are not collected.
func ttyPending(*os.File) func() (string, error) { return nil }

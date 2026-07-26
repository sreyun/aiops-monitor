//go:build !windows

package main

import "fmt"

func runSendSASOnce() error {
	return fmt.Errorf("--send-sas 仅支持 Windows")
}

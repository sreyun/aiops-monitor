//go:build !windows && !linux && !darwin

package main

import (
	"fmt"
	"image"
	"runtime"
)

func openDeskCapture() (deskCapture, error) {
	return nil, fmt.Errorf("web desktop capture not supported on %s yet", runtime.GOOS)
}

func openDeskInput() (deskInput, error) {
	return nil, fmt.Errorf("web desktop input not supported on %s yet", runtime.GOOS)
}

func deskGOOS() string { return runtime.GOOS }

func deskH264Usable() bool       { return false }
func deskPreferredCodec() string { return "" }
func deskAVFScreenIndex() int    { return -1 }

func deskKeyToVK(key, code string) int { return 0 }

func deskClipboardGet() (string, error) {
	return "", fmt.Errorf("clipboard unsupported on %s", runtime.GOOS)
}

func deskClipboardSet(text string) error {
	return fmt.Errorf("clipboard unsupported on %s", runtime.GOOS)
}

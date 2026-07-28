//go:build !windows

package main

func windowsNeedsLegacyAgentBuild() bool { return false }

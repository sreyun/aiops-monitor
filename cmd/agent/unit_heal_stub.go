//go:build !linux

package main

// ensureLinuxAgentUnitPrivileges is a no-op outside Linux.
func ensureLinuxAgentUnitPrivileges() {}

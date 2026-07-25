//go:build !windows

package main

func moduleHyperVPower(args map[string]string) ([]byte, int) {
	return []byte("hyperv_power 仅支持 Windows Hyper-V 宿主机"), 1
}

func moduleHyperVSet(args map[string]string) ([]byte, int) {
	return []byte("hyperv_set 仅支持 Windows Hyper-V 宿主机"), 1
}

//go:build windows

package main

import (
	"fmt"
	"strings"
)

// inspectWindowsFQDN resolves the DNS hostname without invoking Linux-style
// `hostname -f` (which often hits Git/MSYS hostname.exe and returns usage junk).
func inspectWindowsFQDN(fallback string) string {
	out := strings.TrimSpace(string(cmdOutRaw(5, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; [System.Net.Dns]::GetHostEntry('LocalHost').HostName")))
	out = sanitizeInspectField(out)
	if out == "" || looksLikeCommandUsage(out) {
		return fallback
	}
	// Reject multi-word blobs (error text); a real FQDN/hostname is one token.
	if len(strings.Fields(out)) != 1 {
		return fallback
	}
	return out
}

// inspectWindowsOSIdentity returns a human OS label and an NT kernel version
// (e.g. "10.0.26200.8875"), never the localized `ver` banner as kernel.
func inspectWindowsOSIdentity() (pretty, kernel string) {
	pretty = "Windows"
	kernel = "windows"
	script := `
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'SilentlyContinue'
$o = Get-CimInstance Win32_OperatingSystem
$cv = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
Write-Output ("CAPTION=" + [string]$o.Caption)
Write-Output ("DISPLAY=" + [string]$cv.DisplayVersion)
Write-Output ("BUILD=" + [string]$cv.CurrentBuild)
Write-Output ("MAJOR=" + [string]$cv.CurrentMajorVersionNumber)
Write-Output ("MINOR=" + [string]$cv.CurrentMinorVersionNumber)
Write-Output ("UBR=" + [string]$cv.UBR)
`
	raw := string(cmdOutRaw(8, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script))
	vals := map[string]string{}
	for _, ln := range strings.Split(raw, "\n") {
		ln = strings.TrimSpace(strings.TrimSuffix(ln, "\r"))
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		vals[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	caption := sanitizeInspectField(vals["CAPTION"])
	display := sanitizeInspectField(vals["DISPLAY"])
	build := sanitizeInspectField(vals["BUILD"])
	major := sanitizeInspectField(vals["MAJOR"])
	minor := sanitizeInspectField(vals["MINOR"])
	ubr := sanitizeInspectField(vals["UBR"])

	if caption != "" {
		pretty = caption
		if display != "" {
			pretty += " " + display
		}
		if build != "" {
			pretty += " (" + build + ")"
		}
	} else if ver := cmdOut(3, "cmd", "/c", "ver"); ver != "" {
		pretty = ver
	}

	if major == "" {
		major = "10"
	}
	if minor == "" {
		minor = "0"
	}
	if build != "" {
		kernel = fmt.Sprintf("%s.%s.%s", major, minor, build)
		if ubr != "" {
			kernel += "." + ubr
		}
	} else {
		kernel = major + "." + minor
	}
	return pretty, kernel
}

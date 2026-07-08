package main

import (
	"encoding/base64"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// genQRDataURI generates a QR code PNG for the given text and returns it as a
// base64 data URI suitable for use in an <img src="..."> attribute.
//
// Uses EC level Medium (matches Google Authenticator expectations) and a
// 256×256 px canvas — large enough for phone cameras to focus reliably while
// keeping the payload under ~5 KB.
func genQRDataURI(text string) (string, error) {
	png, err := qrcode.Encode(text, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("QR encode: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

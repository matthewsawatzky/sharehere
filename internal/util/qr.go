package util

import (
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

func PrintTerminalQR(value string) {
	qr, err := qrcode.New(value, qrcode.Medium)
	if err != nil {
		return
	}
	fmt.Println(qr.ToSmallString(false))
}

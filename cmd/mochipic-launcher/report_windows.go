package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func reportError(err error) {
	fmt.Fprintln(os.Stderr, err)
	if diagnosticInvocation() {
		return
	}
	message, _ := windows.UTF16PtrFromString("MochiPic could not start.\n\n" + err.Error())
	title, _ := windows.UTF16PtrFromString("MochiPic")
	_, _ = windows.MessageBox(0, message, title, windows.MB_OK|windows.MB_ICONERROR)
}

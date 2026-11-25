//go:build !windows

package main

import (
	"fmt"
	"os"
)

func reportError(err error) {
	fmt.Fprintln(os.Stderr, err)
}

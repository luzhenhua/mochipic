//go:build !windows

package main

import "os"

func updateCommand(string) (bool, error) { return false, nil }

func lockRuntime(*os.Root) (func(), error) { return func() {}, nil }

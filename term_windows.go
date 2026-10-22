package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func isTerminal() bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil
}

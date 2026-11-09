package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// attachConsole gives a run that has arguments the terminal that started it.
// The app is built as a GUI program (-H=windowsgui), so a double-click, the
// update restart and the game launcher open no terminal. That leaves flags
// that print, such as -list-windows and -record, without output, so a run with
// arguments (they come from a script or a person at a terminal) attaches to the
// parent's console. A std handle the parent already redirected to a file or a
// pipe is left as it is.
func attachConsole() {
	if len(os.Args) < 2 {
		return
	}
	std := []struct {
		id   uint32
		name string
		file **os.File
		flag int
	}{
		{windows.STD_INPUT_HANDLE, "CONIN$", &os.Stdin, os.O_RDONLY},
		{windows.STD_OUTPUT_HANDLE, "CONOUT$", &os.Stdout, os.O_WRONLY},
		{windows.STD_ERROR_HANDLE, "CONOUT$", &os.Stderr, os.O_WRONLY},
	}
	missing := make([]bool, len(std))
	anyMissing := false
	for i, s := range std {
		h, err := windows.GetStdHandle(s.id)
		missing[i] = err != nil || h == 0 || h == windows.InvalidHandle
		anyMissing = anyMissing || missing[i]
	}
	if !anyMissing {
		return
	}
	const attachParentProcess = ^uint32(0)
	attach := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	if r, _, _ := attach.Call(uintptr(attachParentProcess)); r == 0 {
		return // no parent console, as when started from Explorer
	}
	for i, s := range std {
		if !missing[i] {
			continue
		}
		if f, err := os.OpenFile(s.name, s.flag, 0); err == nil {
			*s.file = f
		}
	}
}

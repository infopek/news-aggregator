//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: process-control <pid-file> <stop-file> <executable>")
		os.Exit(2)
	}
	command := exec.Command(os.Args[3])
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start application:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[1], []byte(fmt.Sprint(command.Process.Pid)), 0o600); err != nil {
		_ = command.Process.Kill()
		fmt.Fprintln(os.Stderr, "write pid file:", err)
		os.Exit(1)
	}
	for {
		if _, err := os.Stat(os.Args[2]); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(command.Process.Pid)); err != nil {
		_ = command.Process.Kill()
		fmt.Fprintln(os.Stderr, "send console control event:", err)
		os.Exit(1)
	}
	if err := command.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, "application exit:", err)
		os.Exit(1)
	}
}

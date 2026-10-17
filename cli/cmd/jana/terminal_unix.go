//go:build unix

package main

import "golang.org/x/sys/unix"

func termInputReady(fd int) bool {
	p := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, _ := unix.Poll(p, 100)
	return n > 0 && p[0].Revents&unix.POLLIN != 0
}

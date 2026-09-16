//go:build !windows

package main

import "syscall"

func execute(program string, args, env []string) error {
	return syscall.Exec(program, append([]string{program}, args...), env)
}

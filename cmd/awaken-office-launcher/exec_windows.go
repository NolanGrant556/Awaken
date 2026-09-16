package main

import (
	"os"
	"os/exec"
)

func execute(program string, args, env []string) error {
	command := exec.Command(program, args...)
	command.Env = env
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

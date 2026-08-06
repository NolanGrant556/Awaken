//go:build !windows

package main

import "errors"

func run() error {
	return errors.New("the native network proxy observer is only packaged on Windows")
}

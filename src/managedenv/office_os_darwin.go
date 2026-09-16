package managedenv

import "syscall"

func officeHostOSVersion() (string, error) { return syscall.Sysctl("kern.osproductversion") }

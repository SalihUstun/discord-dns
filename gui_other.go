//go:build !darwin

package main

func guiLaunch() error {
	return installCmd(nil)
}

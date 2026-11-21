//go:build !darwin

package sysutil

func readOnly(string) bool { return false }

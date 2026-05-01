//go:build !unix

package main

func detachStdio(logPath string) error { return nil }

package main

import (
	"os"
	"testing"
)

func TestDevNullIsNotTerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = saved }()
	if isTerminal() {
		t.Fatal("/dev/null terminal sanıldı; Finder'dan açılınca pencere çıkmaz")
	}
}

//go:build ignore

package main

import (
	"io"
	"os"
	"path/filepath"
)

// Cross-platform stand-in for `cp` — Windows CI/dev boxes lack GNU cp.
func main() {
	src := filepath.Join("..", "..", "config.example.yaml")
	dst := "config_example.yaml"
	in, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		panic(err)
	}
}

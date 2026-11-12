//go:build !unix

package release

import "os/exec"

func browserCommand(*exec.Cmd) func() { return func() {} }

func prioritizeCommand(*exec.Cmd) {}

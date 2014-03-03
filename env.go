package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Environment struct {
	GoPaths []string
}

func NewEnvironment(paths ...string) *Environment {
	return &Environment{
		GoPaths: paths,
	}
}

func (e *Environment) Command(name string, arg ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(name, arg...)
	// TODO: might want to use multiple GOPATH
	// so go get checkouts are shared
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "GOPATH=") || strings.HasPrefix(env, "GOBIN=") {
			continue
		}
		cmd.Env = append(cmd.Env, env)
	}
	cmd.Env = append(cmd.Env, "GOPATH="+strings.Join(e.GoPaths, fmt.Sprintf("%c", filepath.ListSeparator)))
	return
}

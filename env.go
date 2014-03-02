package main

import (
	"os"
	"os/exec"
	"strings"
)

type Environment struct {
	Path string
}

func NewEnvironment(path string) *Environment {
	return &Environment{path}
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
	cmd.Env = append(cmd.Env, "GOPATH="+e.Path)
	return
}

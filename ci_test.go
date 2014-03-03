package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMakeTree(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("couldn't find filename")
	}
	dir := filepath.Dir(filename)
	dirs, err := makeTree(dir)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%v", dirs)
	if files, ok := dirs[dir]; ok {
		fm := make(map[string]bool)
		for _, file := range files {
			fm[file] = true
		}
		if !fm[filepath.Base(filename)] {
			t.Errorf("%v not found in files", filepath.Base(filename))
		}
	} else {
		t.Errorf("%v not found in map", dir)
	}
}

func TestExecError(t *testing.T) {
	c := exec.Command("fdsfdfsdsdfsdfsdfsd")
	err := c.Run()
	switch err := err.(type) {
	case nil:
		t.Error("got nil, expecting error")
	case *exec.Error:
		t.Log(err)
	case *exec.ExitError:
		t.Error("got exit error")
	default:
		t.Errorf("got %v", err)
	}
}

func TestVetOutParse(t *testing.T) {
	out := "search.go:241: range variable domain enclosed by function\n" +
		"ci.go:34:2: struct field tag `json\"owner\"` not compatible with reflect.StructTag.Get\n"
	msgs := parseVetOut("", bytes.NewReader([]byte(out)))
	if len(msgs) != 2 {
		t.Error("expecting 2 messages")
	}
	// expect lineNumber+1 to put comment under the line in github ui
	if msgs[0].Line != 242 {
		t.Errorf("expecting line 242")
	}
	if msgs[1].Line != 35 {
		t.Errorf("expecting line 35")
	}
}

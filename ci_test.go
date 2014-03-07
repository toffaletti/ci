package main

import (
	_ "github.com/golang/glog"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"
)

var rootTests = []struct {
	in  string
	out string
}{
	{"https://github.com/toffaletti/ci", "src/github.com/toffaletti/ci"},
	{"https://github.com/toffaletti/ci.git", "src/github.com/toffaletti/ci"},
	{"https://example.com/toffaletti/ci.git", "src/example.com/toffaletti/ci.git"},
}

var checkTests = []struct {
	name     string
	expected []codeMessage
}{
	{
		"badfmt",
		[]codeMessage{
			{File: "main.go", Line: 0, Msg: "needs gofmt", Ok: false},
			{File: "", Line: 0, Msg: "# _\n./main.go:4: i declared and not used\n", Ok: false},
		},
	},
	{
		"vetfail",
		[]codeMessage{
			{File: "", Line: 0, Msg: "go vet errors:\nfoo.go:4: struct field tag `json:bad\"` not compatible with reflect.StructTag.Get\nexit status 1\n", Ok: false},
			{File: "", Line: 0, Msg: "?   \t_\t[no test files]\n", Ok: true},
		},
	},
	{
		"missing",
		nil,
	},
	{
		"allok",
		[]codeMessage{
			{File: "", Line: 0, Msg: "ok  \t_\t\tcoverage: 100.0% of statements\n", Ok: true},
		},
	},
	{
		"parsefail",
		[]codeMessage{
			{File: "", Line: 0, Msg: "go vet errors:\nvet: foo.go: foo.go:3:7: expected '(', found newline\nvet: no files checked\nexit status 1\n", Ok: false},
			{File: "", Line: 0, Msg: "/foo.go:3:7: expected '(', found newline", Ok: false},
			{File: "", Line: 0, Msg: "# _\n./foo.go:3: syntax error: unexpected semicolon or newline, expecting (\n", Ok: false},
		},
	},
	{
		"onlyfmt",
		[]codeMessage{
			{File: "hi.go", Line: 0, Msg: "needs gofmt", Ok: false},
			{File: "", Line: 0, Msg: "?   \t_\t[no test files]\n", Ok: true},
		},
	},
}

func TestCheck(t *testing.T) {
	var timeFilterRe = regexp.MustCompile(`[0-9]+\.[0-9]+s`)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("couldn't find filename")
	}
	for i, tt := range checkTests {
		e := NewTestEnv(filepath.Join(filepath.Dir(filename), "_testdata", tt.name))
		e.Check()
		t.Logf("reports: %#v", e.reports)
		// filter out test timings
		for i, r := range e.reports {
			e.reports[i].Msg = timeFilterRe.ReplaceAllString(r.Msg, "")
		}
		if !reflect.DeepEqual(e.reports, tt.expected) {
			t.Errorf("%v. expected: %v got: %v", i, tt.expected, e.reports)
		}
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

func TestRootForUrl(t *testing.T) {
	for i, tt := range rootTests {
		out := rootForUrl("", tt.in)
		if tt.out != out {
			t.Errorf("%v. expected %v got %v", i, tt.out, out)
		}
	}
}

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

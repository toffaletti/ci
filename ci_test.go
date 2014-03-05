package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"
)

var checkTests = []struct {
	name     string
	expected []codeMessage
}{
	{
		"badfmt",
		[]codeMessage{
			{File: "main.go", Line: 4, Msg: "needs gofmt", Ok: false},
			{File: "", Line: 0, Msg: "# _\n./main.go:4: i declared and not used\n", Ok: false},
		},
	},
	{
		"vetfail",
		[]codeMessage{
			{File: "foo.go", Line: 4, Msg: "struct field tag `json:bad\"` not compatible with reflect.StructTag.Get", Ok: false},
			{File: "", Line: 0, Msg: "?   \t_\t[no test files]\n", Ok: true}},
	},
	{
		"missing",
		nil,
	},
	{
		"allok",
		[]codeMessage{
			{File: "", Line: 0, Msg: "PASS\ncoverage: 100.0% of statements\nok  \t_\t\n", Ok: true},
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
	if msgs[0].Line != 241 {
		t.Errorf("expecting line 241")
	}
	if msgs[1].Line != 34 {
		t.Errorf("expecting line 34")
	}
}

func TestBuildOutParse(t *testing.T) {
	out := `
src/github.com/rlee/ml.git/optimizers/logistic_regression_test.go:11: AssertMatrix redeclared in this block
    previous declaration at src/github.com/rlee/ml.git/optimizers/linear_regression_test.go:11
src/github.com/rlee/ml.git/optimizers/logistic_regression_test.go:22: AssertOptimizerWeights redeclared in this block
    previous declaration at src/github.com/rlee/ml.git/optimizers/linear_regression_test.go:22
src/github.com/rlee/ml.git/optimizers/logistic_regression_test.go:45: generateTestData redeclared in this block
    previous declaration at src/github.com/rlee/ml.git/optimizers/linear_regression_test.go:45
src/github.com/rlee/ml.git/optimizers/logistic_regression_test.go:69: TestExactSlope redeclared in this block
    previous declaration at src/github.com/rlee/ml.git/optimizers/linear_regression_test.go:69
src/github.com/rlee/ml.git/optimizers/logistic_regression_test.go:78: TestExactSlopeWithOffset redeclared in this block
    previous declaration at src/github.com/rlee/ml.git/optimizers/linear_regression_test.go:78
    `
	msgs := parseBuildOut("src/github.com/rlee/ml.git/", out)
	t.Logf("msgs: %v", msgs)
	if len(msgs) != 1 {
		t.Errorf("expecting one msg, got %v", len(msgs))
	}
	// TODO: this test is bad
}

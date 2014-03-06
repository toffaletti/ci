package bar

import "testing"

func TestBar(t *testing.T) {
	if !Bar() {
		t.Error("fail")
	}
}

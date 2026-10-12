package ui

import (
	"reflect"
	"testing"
)

func TestParseSelection(t *testing.T) {
	got, err := ParseSelection("1 3, 5-7", 8)
	if err != nil || !reflect.DeepEqual(got, []int{0, 2, 4, 5, 6}) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := ParseSelection("9", 8); err == nil {
		t.Error("out of range should fail")
	}
	if _, err := ParseSelection("x", 8); err == nil {
		t.Error("garbage should fail")
	}
}

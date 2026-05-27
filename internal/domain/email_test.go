package domain

import "testing"

func TestValidCategory(t *testing.T) {
	if !ValidCategory(CategoryUnwanted) {
		t.Fatal("expected unwanted to be valid")
	}
	if ValidCategory(Category("not_real")) {
		t.Fatal("expected unknown category to be invalid")
	}
}

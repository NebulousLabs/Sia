package build

import "testing"

// didPanic returns true if fn panicked.
func didPanic(fn func()) (p bool) {
	defer func() {
		p = (recover() != nil)
	}()
	fn()
	return
}

// TestSelect tests the Select function. Since we can't change the Release
// constant during testing, we can only test the "testing" branches.
func TestSelect(t *testing.T) {
	var v Var
	if !didPanic(func() { Select(v) }) {
		t.Fatal("Select should panic with all nil fields")
	}

	v.Standard = 0
	if !didPanic(func() { Select(v) }) {
		t.Fatal("Select should panic with some nil fields")
	}

	v = Var{
		Standard: 0,
		Dev:      0,
		Testing:  0,
	}
	if didPanic(func() { Select(v) }) {
		t.Fatal("Select should not panic with valid fields")
	}

	if !didPanic(func() { _ = Select(v).(string) }) {
		t.Fatal("improper type assertion should panic")
	}
}

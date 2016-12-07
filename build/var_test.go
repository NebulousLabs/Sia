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
		t.Error("Select should panic with all nil fields")
	}

	v.Standard = 0
	if !didPanic(func() { Select(v) }) {
		t.Error("Select should panic with some nil fields")
	}

	v = Var{
		Standard: 0,
		Dev:      0,
		Testing:  0,
	}
	if didPanic(func() { Select(v) }) {
		t.Error("Select should not panic with valid fields")
	}

	if !didPanic(func() { _ = Select(v).(string) }) {
		t.Error("improper type assertion should panic")
	}
	// should fail even if types are convertible
	type myint int
	if !didPanic(func() { _ = Select(v).(myint) }) {
		t.Error("improper type assertion should panic")
	}

	v.Standard = "foo"
	if !didPanic(func() { Select(v) }) {
		t.Error("Select should panic if field types do not match")
	}

	// Even though myint is convertible to int, it is not *assignable*. That
	// means that this code will panic, as checked in a previous test:
	//
	// _ = Select(v).(myint)
	//
	// This is important because users of Select may assume that type
	// assertions only require convertibility. To guard against this, we
	// enforce that all Var fields must be assignable to each other; otherwise
	// a type assertion may succeed for certain Release constants and fail for
	// others.
	v.Standard = myint(0)
	if !didPanic(func() { Select(v) }) {
		t.Error("Select should panic if field types are not mutually assignable")
	}
}

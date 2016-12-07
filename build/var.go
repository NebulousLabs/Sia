package build

import "reflect"

// A Var represents a variable whose value depends on which Release is being
// compiled. None of the fields may be nil, and all fields must have the same
// type.
type Var struct {
	Standard interface{}
	Dev      interface{}
	Testing  interface{}
	// prevent unkeyed literals
	_ struct{}
}

// Select returns the field of v that corresponds to the current Release.
//
// Since the caller typically makes a type assertion on the result, it is
// important to point out that type assertions are stricter than conversions.
// Specifically, you cannot write:
//
//   type myint int
//   Select(Var{0, 0, 0}).(myint)
//
// Because 0 will be interpreted as an int, which is not assignable to myint.
// Instead, you must explicitly cast each field in the Var, or cast the return
// value of Select after the type assertion. The former is preferred.
func Select(v Var) interface{} {
	if v.Standard == nil || v.Dev == nil || v.Testing == nil {
		panic("nil value in build variable")
	}
	st, dt, tt := reflect.TypeOf(v.Standard), reflect.TypeOf(v.Dev), reflect.TypeOf(v.Testing)
	if !dt.AssignableTo(st) || !tt.AssignableTo(st) {
		// NOTE: we use AssignableTo instead of the more lenient ConvertibleTo
		// because type assertions require the former.
		panic("build variables must have a single type")
	}
	switch Release {
	case "standard":
		return v.Standard
	case "dev":
		return v.Dev
	case "testing":
		return v.Testing
	default:
		panic("unrecognized Release: " + Release)
	}
}

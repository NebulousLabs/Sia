package build

// A Var represents a variable whose value depends on which Release is being
// compiled. None of the fields may be nil, and all fields must have the same
// underlying type.
type Var struct {
	Standard interface{}
	Dev      interface{}
	Testing  interface{}
}

// Select returns the field of v that corresponds to the current Release.
func Select(v Var) interface{} {
	if v.Standard == nil || v.Dev == nil || v.Testing == nil {
		panic("nil value in build variable")
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

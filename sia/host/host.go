package host

type Host interface {
}

type BasicHost struct {
}

func New() BasicHost {
	return BasicHost{}
}

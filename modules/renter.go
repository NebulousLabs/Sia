package modules

type RentFileParameters struct {
	Filepath       string
	Nickname       string
	TotalPieces    int
	RequiredPieces int
	OptimalPieces  int
}

type RentSmallFileParameters struct {
	FullFile       []byte
	Nickname       string
	TotalPieces    int
	RequiredPieces int
	OptimalPieces  int
}

type RentInfo struct {
	Files []string
}

type Renter interface {
	Download(nickname, filepath string) error
	Info() (RentInfo, error)
	Rename(currentName, newName string) error
	RentFile(RentFileParameters) error

	RentSmallFile(RentSmallFileParameters) error
}

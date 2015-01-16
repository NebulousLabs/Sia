package components

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
	RentInfo() (RentInfo, error)
	RenameFile(currentName, newName string) error
	RentFile(RentFileParameters) error

	RentSmallFile(RentSmallFileParameters) error
}

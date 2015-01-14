package components

type RentFileParameters struct {
	Filepath       string
	Nickname       string
	TotalPieces    int
	RequiredPieces int
	OptimalPieces  int
}

type Renter interface {
	RenameFile(currentName, newName string) error
	RentFile(RentFileParameters) error
}

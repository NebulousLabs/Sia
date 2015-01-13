package components

type Renter interface {
	RenameFile(currentName, newName string) error
	RentFile(filename, nickname string, totalPieces, requiredPieces, optimalRecoveryPieces int) error
}

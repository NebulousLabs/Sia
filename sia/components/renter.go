package components

type Renter interface {
	RentFile(filename, nickname string, totalPieces, requiredPieces, optimalRecoveryPieces int) error
}

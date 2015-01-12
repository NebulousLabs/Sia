package components

type Renter interface {
	RentFile(filename string, totalPieces, requiredPieces, optimalRecoveryPieces int) error
}

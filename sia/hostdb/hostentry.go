package hostdb

import (
	"math"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type HostEntry struct {
	ID          string
	IPAddress   network.Address
	MinFilesize uint64
	MaxFilesize uint64
	MinDuration consensus.BlockHeight
	MaxDuration consensus.BlockHeight
	Window      consensus.BlockHeight
	Tolerance   uint64
	Price       consensus.Currency
	Burn        consensus.Currency
	Freeze      consensus.Currency
	CoinAddress consensus.CoinAddress
}

// host.Weight() determines the weight of a specific host, which is:
//
//		Freeze * Burn / square(Price).
//
// Freeze has to be linear, because any non-linear freeze will invite sybil
// attacks.
//
// For now, Burn is also linear because an increased burn means increased risk
// for the host (Freeze on the other hand has no risk). It might be better to
// make burn grow sublinearly, such as taking sqrt(Burn) or burn^(4/5).
//
// We take the square of the price to heavily emphasize hosts that have a low
// price. This is also a bit simplistic however, because we're not sure what
// the host might be charging for bandwidth.
func (h *HostEntry) Weight() consensus.Currency {
	// adjustedBurn := math.Sqrt(float64(h.Burn))
	adjustedBurn := float64(h.Burn)
	adjustedFreeze := float64(h.Freeze)
	adjustedPrice := math.Sqrt(float64(h.Price))

	weight := adjustedFreeze * adjustedBurn / adjustedPrice
	return consensus.Currency(weight)
}

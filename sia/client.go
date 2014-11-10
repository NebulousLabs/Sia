package sia

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type Host struct {
	IPAddress string
	Freeze    Currency
	Burn      Currency
	Price     Currency
	Duration  BlockHeight
}

// Wallet.ClientFundFileContract() takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (w *Wallet) ClientFundFileContract(params *FileContractParameters, state *State) (err error) {
	// Scan the blockchain for outputs.
	w.Scan(state)

	// Add money to the transaction to fund the client's portion of the contract fund.
	err = w.FundTransaction(params.ClientContribution, &params.Transaction)
	if err != nil {
		return
	}

	return
}

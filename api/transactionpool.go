package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/crypto"

	"github.com/julienschmidt/httprouter"
)

// transactionpoolGetTransactionHandler returns the transaction in the tpool
// with the id passed to the handler, or an error if that transaction does not
// exist in the tpool.
func (api *API) transactionpoolGetTransactionHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	txidStr := ps.ByName("id")
	txid := new(crypto.Hash)
	err := txid.LoadString(txidStr)
	if err != nil {
		WriteError(w, Error{"error decoding transaction id:" + err.Error()}, http.StatusBadRequest)
		return
	}

	txns := api.tpool.TransactionList()
	for _, txn := range txns {
		if crypto.Hash(txn.ID()) == *txid {
			WriteJSON(w, txn)
			return
		}
	}

	WriteError(w, Error{"transaction not found in tpool"}, http.StatusBadRequest)
}

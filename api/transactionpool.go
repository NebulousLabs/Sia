package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"

	"github.com/julienschmidt/httprouter"
)

type (
	// TpoolRawGET contains the requested transaction encoded to the raw
	// format, along with the id of that transaction.
	TpoolRawGET struct {
		ID          types.TransactionID `json:"id"`
		Parents     []types.Transaction `json:"parents"`
		Transaction types.Transaction   `json:"transaction"`
	}
)

// transactionpoolRawHandlerGET will provide the raw byte representation of a
// transaction that matches the input id.
func (api *API) transactionpoolRawHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	txidStr := ps.ByName("id")
	txid := new(crypto.Hash)
	err := txid.LoadString(txidStr)
	if err != nil {
		WriteError(w, Error{"error decoding transaction id:" + err.Error()}, http.StatusBadRequest)
		return
	}

	txn, parents, exists := api.tpool.Transaction(txid)
	if !exists {
		WriteError(w, Error{"transaction not found in transaction pool"}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, TpoolRawGET{
		ID:          txid,
		Parents:     parents,
		Transaction: txn,
	})
}

// transactionpoolRawHandlerPOST will provide the raw byte representation of a
// transaction that matches the input id.
func (api *API) transactionpoolRawHandlerPOST(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO: Get parents, get txn, compose into single set, submit set to
	// tpool, return any errors, else success.
}

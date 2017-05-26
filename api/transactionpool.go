package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// TpoolRawGET contains the requested transaction encoded to the raw
	// format, along with the id of that transaction.
	TpoolRawGET struct {
		ID          types.TransactionID `json:"id"`
		Parents     []byte              `json:"parents"`
		Transaction []byte              `json:"transaction"`
	}
)

func decodeTransactionID(txidStr string) (types.TransactionID, error) {
	txid := new(crypto.Hash)
	err := txid.LoadString(txidStr)
	if err != nil {
		return types.TransactionID{}, err
	}
	return types.TransactionID(*txid), nil
}

// transactionpoolRawHandlerGET will provide the raw byte representation of a
// transaction that matches the input id.
func (api *API) tpoolRawHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	txid, err := decodeTransactionID(ps.ByName("id"))
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
		Parents:     encoding.Marshal(parents),
		Transaction: encoding.Marshal(txn),
	})
}

// transactionpoolRawHandlerPOST takes a raw encoded transaction set and posts
// it to the transaction pool, relaying it to the transaction pool's peers
// regardless of if the set is accepted.
func (api *API) tpoolRawHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	rawParents := []byte(req.FormValue("parents"))
	rawTransaction := []byte(req.FormValue("transaction"))
	var parents []types.Transaction
	var txn types.Transaction
	err := encoding.Unmarshal(rawParents, &parents)
	if err != nil {
		WriteError(w, Error{"error decoding parents:" + err.Error()}, http.StatusBadRequest)
		return
	}
	err = encoding.Unmarshal(rawTransaction, &txn)
	if err != nil {
		WriteError(w, Error{"error decoding transaction:" + err.Error()}, http.StatusBadRequest)
		return
	}

	txnSet := append(parents, txn)

	api.tpool.Broadcast(txnSet)
	err = api.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		WriteError(w, Error{"error accepting transaction set:" + err.Error()}, http.StatusBadRequest)
		return
	}

	WriteSuccess(w)
}

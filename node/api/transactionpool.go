package api

import (
	"encoding/base64"
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// TpoolFeeGET contains the current estimated fee
	TpoolFeeGET struct {
		Minimum types.Currency `json:"minimum"`
		Maximum types.Currency `json:"maximum"`
	}

	// TpoolRawGET contains the requested transaction encoded to the raw
	// format, along with the id of that transaction.
	TpoolRawGET struct {
		ID          types.TransactionID `json:"id"`
		Parents     []byte              `json:"parents"`
		Transaction []byte              `json:"transaction"`
	}

	// TpoolConfirmedGET contains information about whether or not
	// the transaction has been seen on the blockhain
	TpoolConfirmedGET struct {
		Confirmed bool `json:"confirmed"`
	}
)

// decodeTransactionID will decode a transaction id from a string.
func decodeTransactionID(txidStr string) (types.TransactionID, error) {
	txid := new(crypto.Hash)
	err := txid.LoadString(txidStr)
	if err != nil {
		return types.TransactionID{}, err
	}
	return types.TransactionID(*txid), nil
}

// tpoolFeeHandlerGET returns the current estimated fee. Transactions with
// fees are lower than the estimated fee may take longer to confirm.
func (api *API) tpoolFeeHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	min, max := api.tpool.FeeEstimation()
	WriteJSON(w, TpoolFeeGET{
		Minimum: min,
		Maximum: max,
	})
}

// tpoolRawHandlerGET will provide the raw byte representation of a
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

// tpoolRawHandlerPOST takes a raw encoded transaction set and posts
// it to the transaction pool, relaying it to the transaction pool's peers
// regardless of if the set is accepted.
func (api *API) tpoolRawHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Try accepting the transactions both as base64 and as clean values.
	//rawParents, err := base64.StdEncoding.DecodeString(req.FormValue("parents"))
	//if err != nil {
	//	rawParents = []byte(req.FormValue("parents"))
	//}
	rawTransaction, err := base64.StdEncoding.DecodeString(req.FormValue("transaction"))
	if err != nil {
		rawTransaction = []byte(req.FormValue("transaction"))
	}

	// Decode the transaction and parents into a transaction set that can be
	// given to the transaction pool.
	//var parents []types.Transaction
	var txn types.Transaction
	//err = encoding.Unmarshal(rawParents, &parents)
	//if err != nil {
	//	WriteError(w, Error{"error decoding parents:" + err.Error()}, http.StatusBadRequest)
	//	return
	//}
	err = encoding.Unmarshal(rawTransaction, &txn)
	//if err == nil {
	//	WriteError(w, Error{"no error, unmarshal ok" + err.Error()}, http.StatusBadRequest)
	//	return
	//}
	if err != nil {
		WriteError(w, Error{"error decoding transaction:" + err.Error()}, http.StatusBadRequest)
		return
	}
	//txnSet := append(parents, txn)
	txnSet := []types.Transaction{txn}
	// Re-broadcast the transactions, so that they are passed to any peers that
	// may have rejected them earlier.
	api.tpool.Broadcast(txnSet)
	err = api.tpool.AcceptTransactionSet(txnSet)
	if err != nil && err != modules.ErrDuplicateTransactionSet {
		WriteError(w, Error{"error accepting transaction set:" + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// tpoolConfirmedGET returns whether the specified transaction has
// been seen on the blockchain.
func (api *API) tpoolConfirmedGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	txid, err := decodeTransactionID(ps.ByName("id"))
	if err != nil {
		WriteError(w, Error{"error decoding transaction id:" + err.Error()}, http.StatusBadRequest)
		return
	}
	confirmed, err := api.tpool.TransactionConfirmed(txid)
	if err != nil {
		WriteError(w, Error{"error fetching transaction status:" + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, TpoolConfirmedGET{
		Confirmed: confirmed,
	})
}

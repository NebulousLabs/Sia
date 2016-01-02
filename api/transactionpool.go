package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// transactionpoolTransactionsHandler handles the API call to get the
// transaction pool trasactions.
func (srv *Server) transactionpoolTransactionsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, srv.tpool.TransactionList())
}

package api

import (
	"net/http"
)

// transactionpoolTransactionsHandler handles the API call to get the
// transaction pool trasactions.
func (srv *Server) transactionpoolTransactionsHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.tpool.TransactionSet())
}

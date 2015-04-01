package main

import (
	"net/http"
)

// transactionpoolTransactionsHandler handles the API call to get the
// transaction pool trasactions.
func (d *daemon) transactionpoolTransactionsHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.tpool.TransactionSet())
}

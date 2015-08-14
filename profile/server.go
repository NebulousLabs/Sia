// +build profile

package profile

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe("localhost:10501", nil)
}

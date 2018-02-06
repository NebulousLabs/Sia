package client

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node/api"
)

// RenterContractsGet requests the /renter/contracts resource
func (c *Client) RenterContractsGet() (rc api.RenterContracts, err error) {
	err = c.Get("/renter/contracts", &rc)
	return
}

// RenterDownloadGet uses the /renter/download endpoint to download a file
func (c *Client) RenterDownloadGet(destination string, offset, length uint64, async bool) (err error) {
	query := fmt.Sprintf("?destination=%v&offset=%v&length=%v&httpresp=false&async=%v",
		destination, offset, length, async)
	err = c.Get("/renter/download"+query, nil)
	return
}

// RenterFilesGet requests the /renter/files resource
func (c *Client) RenterFilesGet() (rf api.RenterFiles, err error) {
	err = c.Get("/renter/files", &rf)
	return
}

// RenterPost uses the /renter endpoint to change the renter's allowance
func (c *Client) RenterPost(allowance modules.Allowance) (err error) {
	values := url.Values{}
	values.Set("funds", allowance.Funds.String())
	values.Set("hosts", strconv.FormatUint(allowance.Hosts, 10))
	values.Set("period", strconv.FormatUint(uint64(allowance.Period), 10))
	values.Set("renewwindow", strconv.FormatUint(uint64(allowance.RenewWindow), 10))
	err = c.Post("/renter", values.Encode(), nil)
	return
}

// RenterUploadPost uses the /renter/upload endpoin to upload a file
func (c *Client) RenterUploadPost(path, siaPath string, dataPieces, parityPieces uint64) (err error) {
	values := url.Values{}
	values.Set("source", path)
	values.Set("datapieces", strconv.FormatUint(dataPieces, 10))
	values.Set("paritypieces", strconv.FormatUint(parityPieces, 10))
	err = c.Post(fmt.Sprintf("/renter/upload%v", siaPath), values.Encode(), nil)
	return
}

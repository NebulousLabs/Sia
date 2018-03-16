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
	err = c.get("/renter/contracts", &rc)
	return
}

// RenterDeletePost uses the /renter/delete endpoint to delete a file.
func (c *Client) RenterDeletePost(siaPath string) (err error) {
	err = c.post(fmt.Sprintf("/renter/delete/%s", siaPath), "", nil)
	return err
}

// RenterDownloadGet uses the /renter/download endpoint to download a file to a
// destination on disk.
func (c *Client) RenterDownloadGet(siaPath, destination string, offset, length uint64, async bool) (err error) {
	query := fmt.Sprintf("%s?destination=%s&offset=%d&length=%d&httpresp=false&async=%v",
		siaPath, destination, offset, length, async)
	err = c.get("/renter/download/"+query, nil)
	return
}

// RenterDownloadsGet requests the /renter/downloads resource
func (c *Client) RenterDownloadsGet() (rdq api.RenterDownloadQueue, err error) {
	err = c.get("/renter/downloads", &rdq)
	return
}

// RenterDownloadHTTPResponseGet uses the /renter/download endpoint to download
// a file and return its data.
func (c *Client) RenterDownloadHTTPResponseGet(siaPath string, offset, length uint64) (resp []byte, err error) {
	query := fmt.Sprintf("%s?offset=%d&length=%d&httpresp=true", siaPath, offset, length)
	resp, err = c.getRawResponse("/renter/download/" + query)
	return
}

// RenterFilesGet requests the /renter/files resource
func (c *Client) RenterFilesGet() (rf api.RenterFiles, err error) {
	err = c.get("/renter/files", &rf)
	return
}

// RenterPostAllowance uses the /renter endpoint to change the renter's allowance
func (c *Client) RenterPostAllowance(allowance modules.Allowance) (err error) {
	values := url.Values{}
	values.Set("funds", allowance.Funds.String())
	values.Set("hosts", strconv.FormatUint(allowance.Hosts, 10))
	values.Set("period", strconv.FormatUint(uint64(allowance.Period), 10))
	values.Set("renewwindow", strconv.FormatUint(uint64(allowance.RenewWindow), 10))
	err = c.post("/renter", values.Encode(), nil)
	return
}

// RenterPostRateLimit uses the /renter endpoint to change the renter's bandwidth rate
// limit.
func (c *Client) RenterPostRateLimit(readBPS, writeBPS int64) (err error) {
	values := url.Values{}
	values.Set("maxdownloadspeed", strconv.FormatInt(readBPS, 10))
	values.Set("maxuploadspeed", strconv.FormatInt(writeBPS, 10))
	err = c.post("/renter", values.Encode(), nil)
	return
}

// RenterStreamGet uses the /renter/stream endpoint to download data as a
// stream.
func (c *Client) RenterStreamGet(siaPath string) (resp []byte, err error) {
	resp, err = c.getRawResponse("/renter/stream/" + siaPath)
	return
}

// RenterUploadPost uses the /renter/upload endpoin to upload a file
func (c *Client) RenterUploadPost(path, siaPath string, dataPieces, parityPieces uint64) (err error) {
	values := url.Values{}
	values.Set("source", path)
	values.Set("datapieces", strconv.FormatUint(dataPieces, 10))
	values.Set("paritypieces", strconv.FormatUint(parityPieces, 10))
	err = c.post(fmt.Sprintf("/renter/upload%v", siaPath), values.Encode(), nil)
	return
}

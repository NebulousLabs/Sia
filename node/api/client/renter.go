package client

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
	siaPath = strings.TrimPrefix(siaPath, "/")
	err = c.post(fmt.Sprintf("/renter/delete/%s", siaPath), "", nil)
	return err
}

// RenterDownloadGet uses the /renter/download endpoint to download a file to a
// destination on disk.
func (c *Client) RenterDownloadGet(siaPath, destination string, offset, length uint64, async bool) (err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	query := fmt.Sprintf("%s?destination=%s&offset=%d&length=%d&httpresp=false&async=%v",
		siaPath, destination, offset, length, async)
	err = c.get("/renter/download/"+query, nil)
	return
}

// RenterDownloadFullGet uses the /renter/download endpoint to download a full
// file.
func (c *Client) RenterDownloadFullGet(siaPath, destination string, async bool) (err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	query := fmt.Sprintf("%s?destination=%s&httpresp=false&async=%v",
		siaPath, destination, async)
	err = c.get("/renter/download/"+query, nil)
	return
}

// RenterClearDownloadPost requests the /renter/downloads/clear/*siapath resource
func (c *Client) RenterClearDownloadPost(siaPath string) (err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	err = c.post(fmt.Sprintf("/renter/downloads/clear/%s", siaPath), "", nil)
	return
}

// RenterClearDownloadsPost requests the /renter/downloads/clear resource
func (c *Client) RenterClearDownloadsPost() (err error) {
	err = c.post("/renter/downloads/clear", "", nil)
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
	siaPath = strings.TrimPrefix(siaPath, "/")
	query := fmt.Sprintf("%s?offset=%d&length=%d&httpresp=true", siaPath, offset, length)
	resp, err = c.getRawResponse("/renter/download/" + query)
	return
}

// RenterFileGet uses the /renter/file/:siapath endpoint to query a file.
func (c *Client) RenterFileGet(siaPath string) (rf api.RenterFile, err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	err = c.get("/renter/file/"+siaPath, &rf)
	return
}

// RenterFilesGet requests the /renter/files resource.
func (c *Client) RenterFilesGet() (rf api.RenterFiles, err error) {
	err = c.get("/renter/files", &rf)
	return
}

// RenterGet requests the /renter resource.
func (c *Client) RenterGet() (rg api.RenterGET, err error) {
	err = c.get("/renter", &rg)
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

// RenterCancelAllowance uses the /renter endpoint to cancel the allowance.
func (c *Client) RenterCancelAllowance() (err error) {
	err = c.RenterPostAllowance(modules.Allowance{})
	return
}

// RenterPricesGet requests the /renter/prices endpoint's resources.
func (c *Client) RenterPricesGet() (rpg api.RenterPricesGET, err error) {
	err = c.get("/renter/prices", &rpg)
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

// RenterRenamePost uses the /renter/rename/:siapath endpoint to rename a file.
func (c *Client) RenterRenamePost(siaPathOld, siaPathNew string) (err error) {
	siaPathOld = strings.TrimPrefix(siaPathOld, "/")
	siaPathNew = strings.TrimPrefix(siaPathNew, "/")
	err = c.post("/renter/rename/"+siaPathOld, "newsiapath=/"+siaPathNew, nil)
	return
}

// RenterSetStreamCacheSizePost uses the /renter endpoint to change the renter's
// streamCacheSize for streaming
func (c *Client) RenterSetStreamCacheSizePost(cacheSize uint64) (err error) {
	values := url.Values{}
	values.Set("streamcachesize", strconv.FormatUint(cacheSize, 10))
	err = c.post("/renter", values.Encode(), nil)
	return
}

// RenterStreamGet uses the /renter/stream endpoint to download data as a
// stream.
func (c *Client) RenterStreamGet(siaPath string) (resp []byte, err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	resp, err = c.getRawResponse("/renter/stream/" + siaPath)
	return
}

// RenterStreamPartialGet uses the /renter/stream endpoint to download a part
// of data as a stream.
func (c *Client) RenterStreamPartialGet(siaPath string, start, end uint64) (resp []byte, err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	resp, err = c.getRawPartialResponse("/renter/stream/"+siaPath, start, end)
	return
}

// RenterUploadPost uses the /renter/upload endpoint to upload a file
func (c *Client) RenterUploadPost(path, siaPath string, dataPieces, parityPieces uint64) (err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	values := url.Values{}
	values.Set("source", path)
	values.Set("datapieces", strconv.FormatUint(dataPieces, 10))
	values.Set("paritypieces", strconv.FormatUint(parityPieces, 10))
	err = c.post(fmt.Sprintf("/renter/upload/%v", siaPath), values.Encode(), nil)
	return
}

// RenterUploadDefaultPost uses the /renter/upload endpoint with default
// redundancy settings to upload a file.
func (c *Client) RenterUploadDefaultPost(path, siaPath string) (err error) {
	siaPath = strings.TrimPrefix(siaPath, "/")
	values := url.Values{}
	values.Set("source", path)
	err = c.post(fmt.Sprintf("/renter/upload/%v", siaPath), values.Encode(), nil)
	return
}

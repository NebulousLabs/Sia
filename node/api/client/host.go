package client

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node/api"
)

// HostParam is a parameter in the host's settings that can be changed via the
// API. It is primarily used as a helper struct to ensure type safety.
type HostParam string

const (
	// HostParamCollateralBudget is the collateral budget of the host in
	// hastings.
	HostParamCollateralBudget = HostParam("collateralbudget")
	// HostParamMaxCollateral is the max collateral of the host in hastings.
	HostParamMaxCollateral = HostParam("maxcollateral")
	// HostParamMinContractPrice is the min contract price in hastings.
	HostParamMinContractPrice = HostParam("mincontractprice")
	// HostParamMinDownloadBandwidthPrice is the min download bandwidth price
	// in hastings/byte.
	HostParamMinDownloadBandwidthPrice = HostParam("mindownloadbandwidthprice")
	// HostParamMinUploadBandwidthPrice is the min upload bandwidth price in
	// hastings/byte.
	HostParamMinUploadBandwidthPrice = HostParam("minuploadbandwidthprice")
	// HostParamCollateral is the host's collateral in hastings/byte/block.
	HostParamCollateral = HostParam("collateral")
	// HostParamMinStoragePrice is the minimum storage price in
	// hastings/byte/block.
	HostParamMinStoragePrice = HostParam("minstorageprice")
	// HostParamAcceptingContracts indicates if the host is accepting new
	// contracts.
	HostParamAcceptingContracts = HostParam("acceptingcontracts")
	// HostParamMaxDuration is the max duration of a contract in blocks.
	HostParamMaxDuration = HostParam("maxduration")
	// HostParamWindowSize is the size of the proof window in blocks.
	HostParamWindowSize = HostParam("windowsize")
	// HostParamMaxDownloadBatchSize is the maximum size of the download batch
	// size in bytes.
	HostParamMaxDownloadBatchSize = HostParam("maxdownloadbatchsize")
	// HostParamMaxReviseBatchSize is the maximum size of the revise batch size.
	HostParamMaxReviseBatchSize = HostParam("maxrevisebatchsize")
	// HostParamNetAddress is the announced netaddress of the host.
	HostParamNetAddress = HostParam("netaddress")
)

// HostAnnouncePost uses the /host/announce endpoint to announce the host to
// the network
func (c *Client) HostAnnouncePost() (err error) {
	err = c.post("/host/announce", "", nil)
	return
}

// HostAnnounceAddrPost uses the /host/anounce endpoint to announce the host to
// the network using the provided address.
func (c *Client) HostAnnounceAddrPost(address modules.NetAddress) (err error) {
	err = c.post("/host/announce", "netaddress="+string(address), nil)
	return
}

// HostContractInfoGet uses the /host/contracts endpoint to get information
// about contracts on the host.
func (c *Client) HostContractInfoGet() (cg api.ContractInfoGET, err error) {
	err = c.get("/host/contracts", &cg)
	return
}

// HostEstimateScoreGet requests the /host/estimatescore endpoint.
func (c *Client) HostEstimateScoreGet(param, value string) (eg api.HostEstimateScoreGET, err error) {
	err = c.get(fmt.Sprintf("/host/estimatescore?%v=%v", param, value), &eg)
	return
}

// HostGet requests the /host endpoint.
func (c *Client) HostGet() (hg api.HostGET, err error) {
	err = c.get("/host", &hg)
	return
}

// HostModifySettingPost uses the /host endpoint to change a param of the host
// settings to a certain value.
func (c *Client) HostModifySettingPost(param HostParam, value interface{}) (err error) {
	err = c.post("/host", string(param)+"="+fmt.Sprint(value), nil)
	return
}

// HostStorageFoldersAddPost uses the /host/storage/folders/add api endpoint to
// add a storage folder to a host
func (c *Client) HostStorageFoldersAddPost(path string, size uint64) (err error) {
	values := url.Values{}
	values.Set("path", path)
	values.Set("size", strconv.FormatUint(size, 10))
	err = c.post("/host/storage/folders/add", values.Encode(), nil)
	return
}

// HostStorageFoldersRemovePost uses the /host/storage/folders/remove api
// endpoint to remove a storage folder from a host.
func (c *Client) HostStorageFoldersRemovePost(path string) (err error) {
	values := url.Values{}
	values.Set("path", path)
	err = c.post("/host/storage/folders/remove", values.Encode(), nil)
	return
}

// HostStorageFoldersResizePost uses the /host/storage/folders/resize api
// endpoint to resize an existing storage folder.
func (c *Client) HostStorageFoldersResizePost(path string, size uint64) (err error) {
	values := url.Values{}
	values.Set("path", path)
	values.Set("newsize", strconv.FormatUint(size, 10))
	err = c.post("/host/storage/folders/resize", values.Encode(), nil)
	return
}

// HostStorageGet requests the /host/storage endpoint.
func (c *Client) HostStorageGet() (sg api.StorageGET, err error) {
	err = c.get("/host/storage", &sg)
	return
}

// HostStorageSectorsDeletePost uses the /host/storage/sectors/delete endpoint
// to delete a sector from the host.
func (c *Client) HostStorageSectorsDeletePost(root crypto.Hash) (err error) {
	err = c.post("/host/storage/sectors/delete/"+root.String(), "", nil)
	return
}

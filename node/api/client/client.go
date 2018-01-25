package client

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/errors"
)

// A Client makes requests to the siad HTTP API.
type Client struct {
	address  string
	password string
}

// NewClient creates a new Client using the provided address and password. If
// password is not the empty string, HTTP basic authentication will be used to
// communicate with the API.
func NewClient(address string, password string) *Client {
	return &Client{
		address:  address,
		password: password,
	}
}

// NewRequest constructs a request to the siad HTTP API, setting the correct
// User-Agent and Basic Auth. The resource path must begin with /.
func (c *Client) NewRequest(method, resource string, body io.Reader) (*http.Request, error) {
	url := "http://" + c.address + resource
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sia-Agent")
	if c.password != "" {
		req.SetBasicAuth("", c.password)
	}
	return req, nil
}

// drainAndClose reads rc until EOF and then closes it. drainAndClose should
// always be called on HTTP response bodies, because if the body is not fully
// read, the underlying connection can't be reused.
func drainAndClose(rc io.ReadCloser) {
	io.Copy(ioutil.Discard, rc)
	rc.Close()
}

// readAPIError decodes and returns an api.Error.
func readAPIError(r io.Reader) error {
	var apiErr api.Error
	if err := json.NewDecoder(r).Decode(&apiErr); err != nil {
		return errors.AddContext(err, "could not read error response")
	}
	return apiErr
}

// Get requests the specified resource. The response, if provided, will be
// decoded into obj. The resource path must begin with /.
func (c *Client) Get(resource string, obj interface{}) error {
	req, err := c.NewRequest("GET", resource, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.AddContext(err, "request failed")
	}
	defer drainAndClose(res.Body)

	if res.StatusCode == http.StatusNotFound {
		return errors.New("API call not recognized: " + resource)
	}

	// If the status code is not 2xx, decode and return the accompanying
	// api.Error.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return readAPIError(res.Body)
	}

	if res.StatusCode == http.StatusNoContent || obj != nil {
		// no reason to read the response
		return nil
	}

	err = json.NewDecoder(res.Body).Decode(&obj)
	if err != nil {
		return errors.AddContext(err, "could not read response")
	}
	return nil
}

// Post makes a POST request to the resource at `resource`, using `data` as the
// request body.  The response, if provided, will be decoded into `obj`.
func (c *Client) Post(resource string, data string, obj interface{}) error {
	req, err := c.NewRequest("POST", resource, strings.NewReader(data))
	if err != nil {
		return err
	}
	// TODO: is this necessary?
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.AddContext(err, "request failed")
	}
	defer drainAndClose(res.Body)

	if res.StatusCode == http.StatusNotFound {
		return errors.New("API call not recognized: " + resource)
	}

	// If the status code is not 2xx, decode and return the accompanying
	// api.Error.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return readAPIError(res.Body)
	}

	if res.StatusCode == http.StatusNoContent || obj != nil {
		// no reason to read the response
		return nil
	}

	err = json.NewDecoder(res.Body).Decode(&obj)
	if err != nil {
		return errors.AddContext(err, "could not read response")
	}
	return nil
}

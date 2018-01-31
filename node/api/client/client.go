package client

import (
	"bytes"
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
	// Address is the API address of the siad server.
	Address string

	// Password must match the password of the siad server.
	Password string

	// UserAgent must match the User-Agent required by the siad server. If not
	// set, it defaults to "Sia-Agent".
	UserAgent string
}

// New creates a new Client using the provided address.
func New(address string) *Client {
	return &Client{
		Address: address,
	}
}

// NewRequest constructs a request to the siad HTTP API, setting the correct
// User-Agent and Basic Auth. The resource path must begin with /.
func (c *Client) NewRequest(method, resource string, body io.Reader) (*http.Request, error) {
	url := "http://" + c.Address + resource
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	agent := c.UserAgent
	if agent == "" {
		agent = "Sia-Agent"
	}
	req.Header.Set("User-Agent", agent)
	if c.Password != "" {
		req.SetBasicAuth("", c.Password)
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

// GetRawResponse requests the specified resource. The response, if provided,
// will be returned in a byte slice
func (c *Client) GetRawResponse(resource string) ([]byte, error) {
	req, err := c.NewRequest("GET", resource, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.AddContext(err, "request failed")
	}
	defer drainAndClose(res.Body)

	if res.StatusCode == http.StatusNotFound {
		return nil, errors.New("API call not recognized: " + resource)
	}

	// If the status code is not 2xx, decode and return the accompanying
	// api.Error.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, readAPIError(res.Body)
	}

	if res.StatusCode == http.StatusNoContent {
		// no reason to read the response
		return []byte{}, nil
	}

	// Return the body
	body := make([]byte, res.ContentLength)
	_, err = io.ReadFull(res.Body, body)

	return body, err
}

// Get requests the specified resource. The response, if provided, will be
// decoded into obj. The resource path must begin with /.
func (c *Client) Get(resource string, obj interface{}) error {
	// Request resource
	data, err := c.GetRawResponse(resource)
	if err != nil {
		return err
	}
	if obj == nil {
		// No need to decode response
		return nil
	}

	// Decode response
	buf := bytes.NewBuffer(data)
	err = json.NewDecoder(buf).Decode(obj)
	if err != nil {
		return errors.AddContext(err, "could not read response")
	}
	return nil
}

// PostRawResponse requests the specified resource. The response, if provided,
// will be returned in a byte slice
func (c *Client) PostRawResponse(resource string, data string) ([]byte, error) {
	req, err := c.NewRequest("POST", resource, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	// TODO: is this necessary?
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.AddContext(err, "request failed")
	}
	defer drainAndClose(res.Body)

	if res.StatusCode == http.StatusNotFound {
		return nil, errors.New("API call not recognized: " + resource)
	}

	// If the status code is not 2xx, decode and return the accompanying
	// api.Error.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, readAPIError(res.Body)
	}

	if res.StatusCode == http.StatusNoContent {
		// no reason to read the response
		return []byte{}, nil
	}

	// Return the body
	body := make([]byte, res.ContentLength)
	_, err = io.ReadFull(res.Body, body)

	return body, err
}

// Post makes a POST request to the resource at `resource`, using `data` as the
// request body. The response, if provided, will be decoded into `obj`.
func (c *Client) Post(resource string, data string, obj interface{}) error {
	// Request resource
	body, err := c.PostRawResponse(resource, data)
	if err != nil {
		return err
	}
	if obj == nil {
		// No need to decode response
		return nil
	}

	// Decode response
	buf := bytes.NewBuffer(body)
	err = json.NewDecoder(buf).Decode(obj)
	if err != nil {
		return errors.AddContext(err, "could not read response")
	}
	return nil
}

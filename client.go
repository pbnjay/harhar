/*
The MIT License (MIT)

Copyright (c) 2014 Stridatum LLC <code@stridatum.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

// Package harhar provides a minimal set of methods and structs to enable
// HAR logging in a go net/http-based application.
package harhar

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client embeds a http.Client and wraps its methods to perform transparent HAR
// logging for every request and response. It contains the properties for the
// root "log" node of the HAR, with Creator, Version, and Comment strings.
type Client struct {
	cli *http.Client

	// Creator describes the source of the logged requests/responses.
	Creator struct {
		// Name defaults to the name of the program (os.Args[0])
		Name string `json:"name"`

		// Version defaults to the current time (formatted as "20060102150405")
		Version string `json:"version"`
	} `json:"creator"`

	// Version defaults to the current time (formatted as "20060102150405")
	Version string `json:"version"`

	// Comment can be added to the log to describe the particulars of this data.
	Comment string `json:"comment,omitempty"`

	// Entries contains all of the Request and Response details that passed
	// through this Client.
	Entries []Entry `json:"entries"`
}

// ClientInterface allows you to dynamically swap in a harhar.Client for
// a http.Client if needed. (Although you'll still need to type-convert to use
// http.Client fields or WriteLog)
type ClientInterface interface {
	Get(url string) (*http.Response, error)
	Head(url string) (*http.Response, error)
	Post(url string, bodyType string, body io.Reader) (*http.Response, error)
	PostForm(url string, data url.Values) (*http.Response, error)
	Do(req *http.Request) (*http.Response, error)
}

func NewClient(client *http.Client) *Client {
	nowVersion := time.Now().Format("20060102150405")
	c := &Client{
		cli:     client,
		Version: nowVersion,
	}
	// add some reasonable defaults
	c.Creator.Name = os.Args[0]
	c.Creator.Version = nowVersion
	return c
}

// WriteLog writes the HAR log format to the filename given, then returns the
// number of bytes.
func (c *Client) WriteLog(filename string) (int, error) {
	data, err := json.Marshal(map[string]*Client{"log": c})
	if err != nil {
		return 0, err
	}
	return len(data), ioutil.WriteFile(filename, data, 0644)
}

///////////////////////////
// wrappers to implement same interface as http.Client
///////////////////////////

// Get works just like http.Client.Get, creating a GET Request and calling Do.
func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Head works just like http.Client.Head, creating a HEAD Request and calling Do.
func (c *Client) Head(url string) (*http.Response, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post works just like http.Client.Post, creating a POST Request with the
// provided body data, setting the content-type to bodyType, and calling Do.
func (c *Client) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", bodyType)
	return c.Do(req)
}

// PostForm works just like http.Client.PostForm, creating a POST Request by
// urlencoding data, setting the content-type appropriately, and calling Do.
func (c *Client) PostForm(url string, data url.Values) (*http.Response, error) {
	return c.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

// Do works by calling http.Client.Do on the wrapped client instance. However,
// it also tracks the request start and end times, and parses elements from the
// request and response data into HAR log Entries.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	var err error
	ent := Entry{}
	ent.Request, err = makeRequest(req)
	if err != nil {
		return nil, err
	}
	ent.Cache = make(map[string]string)

	startTime := time.Now()
	resp, err := c.cli.Do(req)
	finish := time.Now()
	if err != nil {
		return resp, err
	}

	// very hard to get these numbers
	ent.Timings.Send = -1
	ent.Timings.Wait = -1

	if resp.Header.Get("Date") != "" {
		svrReceived, terr := time.Parse(time.RFC1123, resp.Header.Get("Date"))
		if terr != nil {
			svrReceived, terr = time.Parse(time.RFC1123Z, resp.Header.Get("Date"))
		}
		if terr == nil {
			ent.Timings.Wait = int(svrReceived.Sub(startTime).Seconds() * 1000.0)
			ent.Timings.Receive = int(finish.Sub(svrReceived).Seconds() * 1000.0)
			ent.Time = ent.Timings.Wait + ent.Timings.Receive
		}
	}
	if ent.Timings.Wait == -1 {
		ent.Timings.Receive = int(finish.Sub(startTime).Seconds() * 1000.0)
		ent.Time = ent.Timings.Receive
	}

	ent.Start = startTime.Format(time.RFC3339Nano)
	ent.Response, err = makeResponse(resp)

	// add entry to log
	c.Entries = append(c.Entries, ent)
	return resp, err
}

// convert an http.Request to a harhar.Request
func makeRequest(hr *http.Request) (Request, error) {
	r := Request{
		Method:      hr.Method,
		URL:         hr.URL.String(),
		HttpVersion: hr.Proto,
		HeadersSize: -1,
		BodySize:    -1,
	}

	// parse out headers
	r.Headers = make([]NameValuePair, 0, len(hr.Header))
	for name, vals := range hr.Header {
		for _, val := range vals {
			r.Headers = append(r.Headers, NameValuePair{name, val})
		}
	}

	// parse out cookies
	r.Cookies = make([]Cookie, 0, len(hr.Cookies()))
	for _, c := range hr.Cookies() {
		nc := Cookie{
			Name:     c.Name,
			Path:     c.Path,
			Value:    c.Value,
			Domain:   c.Domain,
			Expires:  c.Expires.Format(time.RFC3339Nano),
			HttpOnly: c.HttpOnly,
			Secure:   c.Secure,
		}
		r.Cookies = append(r.Cookies, nc)
	}

	// parse query params
	qp := hr.URL.Query()
	r.QueryParams = make([]NameValuePair, 0, len(qp))
	for name, vals := range qp {
		for _, val := range vals {
			r.QueryParams = append(r.QueryParams, NameValuePair{name, val})
		}
	}

	if hr.Body == nil {
		return r, nil
	}

	// read in all the data and replace the ReadCloser
	bodyData, err := ioutil.ReadAll(hr.Body)
	if err != nil {
		return r, err
	}
	hr.Body.Close()
	hr.Body = ioutil.NopCloser(bytes.NewReader(bodyData))

	r.Body.Content = string(bodyData)
	r.Body.MIMEType = hr.Header.Get("Content-Type")
	if r.Body.MIMEType == "" {
		// default per RFC2616
		r.Body.MIMEType = "application/octet-stream"
	}

	return r, nil
}

// convert an http.Response to a harhar.Response
func makeResponse(hr *http.Response) (Response, error) {
	r := Response{
		StatusCode:  hr.StatusCode,
		StatusText:  hr.Status[4:], // "200 OK" => "OK"
		HttpVersion: hr.Proto,
		HeadersSize: -1,
		BodySize:    -1,
	}

	// parse out headers
	r.Headers = make([]NameValuePair, 0, len(hr.Header))
	for name, vals := range hr.Header {
		for _, val := range vals {
			r.Headers = append(r.Headers, NameValuePair{name, val})
		}
	}

	// parse out cookies
	r.Cookies = make([]Cookie, 0, len(hr.Cookies()))
	for _, c := range hr.Cookies() {
		nc := Cookie{
			Name:     c.Name,
			Path:     c.Path,
			Value:    c.Value,
			Domain:   c.Domain,
			Expires:  c.Expires.Format(time.RFC3339Nano),
			HttpOnly: c.HttpOnly,
			Secure:   c.Secure,
		}
		r.Cookies = append(r.Cookies, nc)
	}

	// TODO: check for redirect URL?

	// read in all the data and replace the ReadCloser
	bodyData, err := ioutil.ReadAll(hr.Body)
	if err != nil {
		return r, err
	}
	hr.Body.Close()
	hr.Body = ioutil.NopCloser(bytes.NewReader(bodyData))
	r.Body.Content = string(bodyData)
	r.Body.Size = len(bodyData)

	r.Body.MIMEType = hr.Header.Get("Content-Type")
	if r.Body.MIMEType == "" {
		// default per RFC2616
		r.Body.MIMEType = "application/octet-stream"
	}

	return r, nil
}

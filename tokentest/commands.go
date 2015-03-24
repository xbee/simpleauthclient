package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

func (c *DaemonCli) readBody(stream io.ReadCloser, statusCode int, err error) ([]byte, int, error) {
	if stream != nil {
		defer stream.Close()
	}
	if err != nil {
		return nil, statusCode, err
	}
	body, err := ioutil.ReadAll(stream)
	if err != nil {
		return nil, -1, err
	}
	return body, statusCode, nil
}

func (c *DaemonCli) HTTPClient() *http.Client {
	return &http.Client{Transport: c.transport}
}

func (c *DaemonCli) encodeData(data interface{}) (*bytes.Buffer, error) {
	params := bytes.NewBuffer(nil)
	if data != nil {
		buf, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		if _, err := params.Write(buf); err != nil {
			return nil, err
		}
	}
	return params, nil
}

func (c *DaemonCli) call(method, path string, data interface{}, passAuthInfo bool) (io.ReadCloser, int, error) {
	params, err := c.encodeData(data)
	if err != nil {
		return nil, -1, err
	}
	req, err := http.NewRequest(method, fmt.Sprintf("/v%s%s", version, path), params)
	if err != nil {
		return nil, -1, err
	}
	if passAuthInfo {
	}
	req.Header.Set("User-Agent", "Daemon-Client/")
	req.URL.Host = c.addr
	req.URL.Scheme = c.scheme
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	} else if method == "POST" {
		req.Header.Set("Content-Type", "text/plain")
	}
	rsp, err := c.HTTPClient().Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, -1, errors.New("connection refused")
		}

		return nil, -1, fmt.Errorf("An error occurred trying to connect: %v", err)
	}

	if rsp.StatusCode < 200 || rsp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return nil, -1, err
		}
		if len(body) == 0 {
			return nil, rsp.StatusCode, fmt.Errorf("Error: request returned %s for API route and version %s, check if the server supports the requested API version", http.StatusText(rsp.StatusCode), req.URL)
		}
		return nil, rsp.StatusCode, fmt.Errorf("Error response from daemon: %s", bytes.TrimSpace(body))
	}

	return rsp.Body, rsp.StatusCode, nil
}

func (c *DaemonCli) stream(method, path string, in io.Reader, stdout, stderr io.Writer, headers map[string][]string) error {
	if (method == "POST" || method == "PUT") && in == nil {
		in = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequest(method, fmt.Sprintf("/v%s%s", version, path), in)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Docker-Client/")
	req.URL.Host = c.addr
	req.URL.Scheme = c.scheme
	if method == "POST" {
		req.Header.Set("Content-Type", "text/plain")
	}

	if headers != nil {
		for k, v := range headers {
			req.Header[k] = v
		}
	}
	resp, err := c.HTTPClient().Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("Cannot connect to the Docker daemon.")
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return fmt.Errorf("Error :%s", http.StatusText(resp.StatusCode))
		}
		return fmt.Errorf("Error: %s", bytes.TrimSpace(body))
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {

	}
	if stdout != nil || stderr != nil {
		// When TTY is ON, use regular copy
		_, err = io.Copy(stdout, resp.Body)
		return err
	}
	return nil
}

func (c *DaemonCli) CmdLogin(args ...string) error {
	data := make(map[string]string)
	fmt.Fprint(c.out, "\n user :")
	buf := make([]byte, 100)

	if n, err := c.in.Read(buf); err != nil {
		return err
	} else if n <= 0 {
		return fmt.Errorf("user is empty")
	}
	data["username"] = string(buf[:strings.Index(string(buf), "\r\n")])
	fmt.Fprint(c.out, "\n password :")
	if n, err := c.in.Read(buf); err != nil {
		return err
	} else if n <= 0 {
		return fmt.Errorf("passwd is empty")
	}
	data["password"] = string(buf[:strings.Index(string(buf), "\r\n")])
	in, err := c.encodeData(data)
	if err != nil {
		return err
	}
	return c.stream("POST", "/token", in, c.out, c.err, nil)
}

func (c *DaemonCli) CmdReset(args ...string) error {
	if len(args) < 1 {
		return fmt.Errorf("Not enough parameters")
	}
	data := make(map[string]string)
	data["password"] = args[0]
	in, err := c.encodeData(data)
	if err != nil {
		return err
	}
	return c.stream("POST", "/reset", in, c.out, c.err, nil)
}

func (c *DaemonCli) CmdVersion(args ...string) error {
	return c.stream("GET", "/version", nil, c.out, c.err, nil)
}

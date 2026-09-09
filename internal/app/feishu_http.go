package app

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// SDK downloads buffer the response before returning; enforce limits before that buffer.
type feishuHTTP struct{ client *http.Client }

func newFeishuHTTP() *feishuHTTP { return &feishuHTTP{client: &http.Client{Timeout: 15 * time.Second}} }
func (c *feishuHTTP) Do(req *http.Request) (*http.Response, error) {
	r, e := c.client.Do(req)
	if e != nil {
		return nil, e
	}
	limit := int64(2 * 1024 * 1024)
	if strings.Contains(req.URL.Path, "/resources/") {
		limit = 20 * 1024 * 1024
	}
	defer r.Body.Close()
	data, e := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(data)) > limit {
		return nil, errors.New("飞书响应超过大小限制")
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return r, nil
}

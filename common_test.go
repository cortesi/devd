package devd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type handlerTester struct {
	t *testing.T
	h http.Handler
}

// Request makes a test request
func (ht *handlerTester) Request(method string, url string, params url.Values) *httptest.ResponseRecorder {
	req, err := http.NewRequest(method, url, strings.NewReader(params.Encode()))
	if err != nil {
		ht.t.Errorf("%v", err)
	}
	if params != nil {
		req.Header.Set(
			"Content-Type",
			"application/x-www-form-urlencoded; param=value",
		)
	}
	w := httptest.NewRecorder()
	ht.h.ServeHTTP(w, req)
	return w
}

// AssertCode asserts that the HTTP return code matches an expected value
func AssertCode(t *testing.T, resp *httptest.ResponseRecorder, code int) {
	if resp.Code != code {
		t.Errorf("Expected code %d, got %d", code, resp.Code)
	}
}

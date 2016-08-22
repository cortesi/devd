package devd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/GeertJohan/go.rice"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/ricetemp"
)

func tFilesystemEndpoint(s string) *filesystemEndpoint {
	e, _ := newFilesystemEndpoint(s)
	return e
}

func tForwardEndpoint(s string) *forwardEndpoint {
	e, _ := newForwardEndpoint(s)
	return e
}

func within(s string, e error) bool {
	s = strings.ToLower(s)
	estr := strings.ToLower(fmt.Sprint(e))
	return strings.Contains(estr, s)
}

var newSpecTests = []struct {
	raw  string
	spec *Route
	err  string
}{
	{
		"/one=two",
		&Route{"", "/one", tFilesystemEndpoint("two")},
		"",
	},
	{
		"/one=two=three",
		&Route{"", "/one", tFilesystemEndpoint("two=three")},
		"",
	},
	{
		"one",
		&Route{"", "/", tFilesystemEndpoint("one")},
		"invalid spec",
	},
	{"=one", nil, "invalid spec"},
	{"one=", nil, "invalid spec"},
	{
		"one/two=three",
		&Route{"one.devd.io", "/two", tFilesystemEndpoint("three")},
		"",
	},
	{
		"one=three",
		&Route{"one.devd.io", "/", tFilesystemEndpoint("three")},
		"",
	},
	{
		"one=http://three",
		&Route{"one.devd.io", "/", tForwardEndpoint("http://three")},
		"",
	},
	{
		"one=localhost:1234",
		nil,
		"Unknown scheme 'localhost': did you mean http or https?: localhost:1234",
	},
	{
		"one=localhost:1234/abc",
		nil,
		"Unknown scheme 'localhost': did you mean http or https?: localhost:1234/abc",
	},
	{
		"one=ws://three",
		nil,
		"Websocket protocol not supported: ws://three",
	},
}

func TestParseSpec(t *testing.T) {
	for i, tt := range newSpecTests {
		s, err := newRoute(tt.raw)
		if tt.spec != nil {
			if err != nil {
				t.Errorf("Test %d, error:\n%s\n", i, err)
				continue
			}
			if !reflect.DeepEqual(s, tt.spec) {
				t.Errorf("Test %d, expecting:\n%s\nGot:\n%s\n", i, tt.spec, s)
				continue
			}
		} else if tt.err != "" {
			if err == nil {
				t.Errorf("Test %d, expected error:\n%s\n", i, tt.err)
				continue
			}
			if !within(tt.err, err) {
				t.Errorf(
					"Test %d, expected error:\n%s\nGot error:%s\n",
					i,
					tt.err,
					err,
				)
				continue
			}
		}
	}
}

func TestForwardEndpoint(t *testing.T) {
	f, err := newForwardEndpoint("http://foo")
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
	rb, err := rice.FindBox("templates")
	if err != nil {
		t.Error(err)
	}
	templates, err := ricetemp.MakeTemplates(rb)
	if err != nil {
		panic(err)
	}

	f.Handler(templates, inject.CopyInject{})

	f, err = newForwardEndpoint("%")
	if err == nil {
		t.Errorf("Expected error, got %s", f)
	}
}

func TestNewRoute(t *testing.T) {
	r, err := newRoute("foo=http://%")
	if err == nil {
		t.Errorf("Expected error, got %s", r)
	}
}

func TestRouteHandler(t *testing.T) {
	var routeHandlerTests = []struct {
		spec string
	}{
		{"/one=two"},
	}
	for i, tt := range routeHandlerTests {
		r, err := newRoute(tt.spec)
		if err != nil {
			t.Errorf(
				"Test %d, unexpected error:\n%s\n",
				i,
				err,
			)
		}

		rb, err := rice.FindBox("templates")
		if err != nil {
			t.Error(err)
		}
		templates, err := ricetemp.MakeTemplates(rb)
		if err != nil {
			panic(err)
		}

		r.Endpoint.Handler(templates, inject.CopyInject{})
	}
}

func TestRouteCollection(t *testing.T) {
	var m = make(RouteCollection)
	_ = m.String()
	err := m.Add("foo=bar")
	if err != nil {
		t.Error(err)
	}
	err = m.Add("foo")
	if err != nil {
		t.Error(err)
	}

	err = m.Add("xxx=bar")
	if err != nil {
		t.Errorf("Set error: %s", err)
	}

	err = m.Add("xxx=bar")
	if err == nil {
		t.Errorf("Expected error, got: %s", m)
	}
}

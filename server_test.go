package devd

import (
	"reflect"
	"testing"

	"github.com/GeertJohan/go.rice"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/ricetemp"
	"github.com/cortesi/termlog"
)

var formatURLTests = []struct {
	tls    bool
	addr   string
	port   int
	output string
}{
	{true, "127.0.0.1", 8000, "https://devd.io:8000"},
	{false, "127.0.0.1", 8000, "http://devd.io:8000"},
	{false, "127.0.0.1", 80, "http://devd.io"},
	{true, "127.0.0.1", 443, "https://devd.io"},
	{false, "127.0.0.1", 443, "http://devd.io:443"},
}

func TestFormatURL(t *testing.T) {
	for i, tt := range formatURLTests {
		url := formatURL(tt.tls, tt.addr, tt.port)
		if url != tt.output {
			t.Errorf("Test %d, expected \"%s\" got \"%s\"", i, tt.output, url)
		}
	}
}

func TestPickPort(t *testing.T) {
	_, err := pickPort("127.0.0.1", 8000, 10000, true)
	if err != nil {
		t.Errorf("Could not bind to any port: %s", err)
	}
	_, err = pickPort("127.0.0.1", 8000, 8000, true)
	if err == nil {
		t.Errorf("Expected not to be able to bind to any port")
	}

}

func fsEndpoint(s string) *filesystemEndpoint {
	e, _ := newFilesystemEndpoint(s)
	return e
}

func TestDevdRouteHandler(t *testing.T) {
	logger := termlog.NewLog()
	logger.Quiet()
	r := Route{"", "/", fsEndpoint("./testdata")}
	templates := ricetemp.MustMakeTemplates(rice.MustFindBox("templates"))
	ci := inject.CopyInject{}

	devd := Devd{LivereloadRoutes: true}
	h := devd.WrapHandler(logger, r.Endpoint.Handler(templates, ci))
	ht := handlerTester{t, h}

	AssertCode(t, ht.Request("GET", "/", nil), 200)
}

func TestDevdHandler(t *testing.T) {
	logger := termlog.NewLog()
	logger.Quiet()
	templates := ricetemp.MustMakeTemplates(rice.MustFindBox("templates"))

	devd := Devd{LivereloadRoutes: true, WatchPaths: []string{"./"}}
	devd.AddRoutes([]string{"./"})
	h, err := devd.Router(logger, templates)
	if err != nil {
		t.Error(err)
	}
	ht := handlerTester{t, h}

	AssertCode(t, ht.Request("GET", "/", nil), 200)
	AssertCode(t, ht.Request("GET", "/nonexistent", nil), 404)
}

func TestGetTLSConfig(t *testing.T) {
	_, err := getTLSConfig("nonexistent")
	if err == nil {
		t.Error("Expected failure, found success.")
	}
	_, err = getTLSConfig("./testdata/certbundle.pem")
	if err != nil {
		t.Errorf("Could not get TLS config: %s", err)
	}
}

var credentialsTests = []struct {
	spec  string
	creds *Credentials
}{
	{"foo:bar", &Credentials{"foo", "bar"}},
	{"foo:", nil},
	{":bar", nil},
	{"foo:bar:voing", &Credentials{"foo", "bar:voing"}},
	{"foo", nil},
}

func TestCredentials(t *testing.T) {
	for i, data := range credentialsTests {
		got, _ := CredentialsFromSpec(data.spec)
		if !reflect.DeepEqual(data.creds, got) {
			t.Errorf("%d: got %v, expected %v", i, got, data.creds)
		}
	}
}

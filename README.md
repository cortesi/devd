
[![Build Status](https://drone.io/github.com/cortesi/devd/status.png)](https://drone.io/github.com/cortesi/devd/latest)

![screenshot](docs/devd-terminal.png "devd in action")

# devd: a local webserver for developers

- Reverse proxy and local file server
- Livereload with no browser addon required for both local files and reverse proxied apps
- Virtual hosting using the dedicated **devd.io** domain
- Logs geared for interactive use: coloring, filters, options to show headers and timing information
- Automatically selects a free port to listen on
- **-o** automatically opens a browser window
- Latency and bandwidth limit simulation


## Quick start examples

Serve the current directory:

```bash
devd .
```

... and open it in the browser:

```bash
devd -o .
```

... and live reload when any file changes:

```bash
devd -lo .
```

Reverse proxy to http://localhost:8080, and livereload when any file in the **src** directory changes:

```bash
devd -w ./src http://localhost:8080
```




## Routes

The devd command takes one or more route specifications as arguments. Routes
have the basic format **root=endpoint**. Roots can be fixed, like
"/favicon.ico", or subtrees, like "/images/" (note the trailing slash).
Endpoints can be filesystem paths or URLs to upstream HTTP servers.

Here's a route that serves the directory *./static* under */assets* on the server:

```
/assets/=./static
```

To use the **devd.io** domain for virtual hosting, you can add a sub-domain to
the root specification. We recognize subdomains by the fact that they don't
start with a leading **/**. This route serves the **/sttic** directory under
**static.devd.io:[port]/assets**, where port is the port your server is bound
to.

```
static/assets=./static
```

All devd.io domains resolve to 127.0.0.1, so you can simply open the URL in
your browser, and it will use the local server.

Reverse proxy specifcations are similar, but the endpoint specification is a
URL. The following serves a local URL from the root **app.devd.io/login**:

```
app/login=http://localhost:8888
```

If the **root** specification is omitted, it is assumed to be "/", i.e. a
pattern matching all paths. So, a simple directory specification serves the
directory tree directly under **devd.io**:

```
devd ./static
```

Similarly, a simple reverse proxy can be started like this:

```
devd http://localhost:8888
```


## Livereload

When livereload is enabled, devd injects a small script into HTML pages. This
then listens for changes over a websocket connection, and reloads resources as
needed. No browser addon is required, and livereload works even for reverse
proxied apps. If only changes to CSS files are seen, devd will only reload
external CSS resources, otherwise a full page reload is done.

##

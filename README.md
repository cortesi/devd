
[![Build Status](https://drone.io/github.com/cortesi/devd/status.png)](https://drone.io/github.com/cortesi/devd/latest)


# devd: a local webserver for developers

![screenshot](docs/devd-terminal.png "devd in action")


# Quick start

Serve the current directory, open it in the browser (**-o**), and livereload when files change (**-l**):

```bash
devd -ol .
```

Reverse proxy to http://localhost:8080, and livereload when any file in the **src** directory changes:

```bash
devd -w ./src http://localhost:8080
```


# Features


### Cross-platform and self-contained

Devd is a single statically compiled binary with no external dependencies, and
is released for OSX, Linux and Windows. Don't want to install Node or Python in
that light-weight Docker instance you're hacking in? Just copy over the devd
binary and be done with it.


### Designed for the terminal

This means no config file, no daemonization, and logs that are designed to be
read in the terminal by a developer. Logs are colorized and log entries span
multiple lines. Devd's logs are detailed, warn about corner cases that other
daemons ignore, and can optionally include things like detailed timing
information and full headers.

To make quickly firing up an instance as simple as possible, devd automatically
chooses an open port to run on (unless it's specified), and can open a browser
window pointing to the daemon root for you (the *-o* flag in the example
above).


### Livereload

When livereload is enabled, devd injects a small script into HTML pages, just
before the closing *head* tag. The script listens for change notifications over
a websocket connection, and reloads resources as needed. No browser addon is
required, and livereload works even for reverse proxied apps. If only changes
to CSS files are seen, devd will only reload external CSS resources, otherwise
a full page reload is done. This serves the current directory with livereload
enabled:

<pre class="terminal">devd -l .</pre>

You can also trigger livereload for files that are not being served, letting
you reload reverse proxied applications when source files change. So, this
command watches the *src* directory tree, and reverse proxies to a locally
running application:

<pre class="terminal">devd -w ./src http://localhost:8888</pre>


### Reverse proxy + static file server + flexible routing

Modern apps tend to be collections of web servers, and devd caters for this
with flexible reverse proxying. You can use devd to overlay a set of services
on a single domain, add livereload to services that don't natively support it,
add throttling and latency simulation to existing services, and so forth.

Here's a more complicated example showing how all this ties together - it
overlays two applications and a tree of static files. Livereload is enabled for
the static files (**-l**) and also triggered whenever source files for reverse
proxied apps change:

<pre class="terminal">
devd -l \
-w ./src/ \
/=http://localhost:8888 \
/api/=http://localhost:8889 \
/static/=./assets
</pre>


### Light-weight virtual hosting

Devd uses a dedicated domain - **devd.io** - to do simple virtual hosting. This
domain and all its subdomains resolves to 127.0.0.1, which we use to set up
virtual hosting without any changes to */etc/hosts* or other local
configuration. Route specifications that don't start with a leading **/** are
taken to be subdomains of **devd.io**. So, the following command serves a
static site from devd.io, and reverse proxies a locally
running app on api.devd.io:

<pre class="terminal">
devd ./static api=http://localhost:8888
</pre>

Check out the docs at [the Github repo](https://github.com/cortesi/devd) for
the full route specification syntax.


### Latency and bandwidth simulation

Want to know what it's like to use your fancy 5mb HTML5 app from a mobile phone
in Botswana? Look up the bandwidth and latency
[here](http://www.cisco.com/c/en/us/solutions/collateral/service-provider/global-cloud-index-gci/CloudIndex_Supplement.html),
and invoke devd like so (making sure to convert form kilobits per second to
kilobytes per second):

<pre class="terminal">devd -d 114 -u 51 -l 75 .</pre>

Devd tries to be reasonably accurate in simulating bandwidth and latency - it
uses a token bucket implementation for throttling, properly handles concurrent
requests, and chunks traffic up so data flow is smooth.


## Routes

The devd command takes one or more route specifications as arguments. Routes
have the basic format **root=endpoint**. Roots can be fixed, like
"/favicon.ico", or subtrees, like "/images/" (note the trailing slash).
Endpoints can be filesystem paths or URLs to upstream HTTP servers.

Here's a route that serves the directory *./static* under */assets* on the server:

```
/assets/=./static
```

All **devd.io** domains resolve to 127.0.0.1, and are used by devd for simple,
rough-and-ready virtual hosting. You can add a sub-domain to the root
specification. We recognize subdomains by the fact that they don't start with a
leading **/**. This route serves the **/sttic** directory under
**static.devd.io:[port]/assets**, where port is the port your server is bound
to.

```
static/assets=./static
```

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

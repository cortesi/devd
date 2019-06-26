# Unreleased

* Improves CORS support. Allows connections with credentials that were
  previously refused. See
  <https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS/Errors/CORSNotSupportingCredentials>

# v0.9: 21 January 2019

* Fix live reload issues on Linux (Delyan Angelov)
* Only inject livereload content if content type is text/html (Mattias Wadman)
* Fix treatment of X-Forwarded-Proto (Marvin Frick)
* Dependency updates and test improvements


# v0.8: 8 January 2018

* Improvements in file change monitoring, fixing a number of bugs and
  reliability issues, and improving the way we handle symlinks (via the
  moddwatch repo).
* Fix handling of the X-Forwarded-Proto header in reverse proxy (thanks to Bernd
  Haug <bernd.haug@xaidat.com>).
* Various other minor fixes and documentation updates.


# v0.7: 8 December 2016

* Add the --notfound flag, which specifies over-rides when the static file sever can't find a file. This is useful for single-page JS app development.
* Improved directory listings, nicer 404 pages.
* X-Forwarded-Proto is now added to reverse proxied requests.


# v0.6: 24 September 2016

* Fix support for MacOS Sierra. This just involved a recompile to fix a compatibility issue between older versions of the Go toolchain and Sierra.
* Fix an issue that caused a slash to be added to some URLs forwarded to reverse proxied hosts.
* livereload: endpoints now run on all domains, fixing livereload on subdomain endpoints.
* livereload: fix support  of IE11 (thanks thomas@houseofcode.io).
* Sort directory list entries (thanks @Schnouki).
* Improved route parsing and clarity - (thanks to @aellerton).


# v0.5: 8 April 2016

* Increase the size of the initial file chunk we inspect or a </head> tag for
livereload injection. Fixes some rare cases where pages with a lot of header
data didn't trigger livereload.
* Request that upstream servers do not return compressed data, allowing
livereload script injection. (thanks Thomas B Homburg <thomas@homburg.dk>)
* Bugfix: Fix recursive file monitoring for static routes


# v0.4: 12 February 2016

* Add support for [modd](https://github.com/cortesi/modd), with the -m flag.
* Add -X flag to set Access-Control-Allow-Origin: * on all responses, allowing
  the use of multiple .devd.io domains in testing.
* Add -L flag, which turns on livereload but doesn't trigger on modification,
  allowing livereload to be driven by external tools.
* Add -C flag to force colour output, even if we're not attachd to a terminal.
* Add -t flag to disable timestamps.
* Silence console errors due to a stupid long-standing Firefox bug.
* Fix throttling of data upload.
* Improve display of content sizes.
* Add distributions for OpenBSD and NetBSD.


# v0.3: 12 November 2015

* -s (--tls) Generate a self-signed certificate, and enable TLS. The cert
  bundle is stored in ~/.devd.cert
* Add the X-Forwarded-Host header to reverse proxied traffic.
* Disable upstream cert validation for reverse proxied traffic. This makes
  using self-signed certs for development easy. Devd shoudn't be used in
  contexts where this might pose a security risk.
* Bugfix: make CSS livereload work in Firefox
* Bugfix: make sure the Host header and SNI host matches for reverse proxied
  traffic.


# v0.2

* -x (--exclude) flag to exclude files from livereload.
* -P (--password) flag for quick HTTP Basic password protection.
* -q (--quiet) flag to suppress all output from devd.
* Humanize file sizes in console logs.
* Improve directory indexes - better formatting, they now also livereload.
* Devd's built-in livereload URLs are now less likely to clash with user URLs.
* Internal 404 pages are now included in logs, timing measurement, and
  filtering.
* Improved heuristics for livereload file change detection. We now handle
  things like transient files created by editors better.
* A Linux ARM build will now be distributed with each release.

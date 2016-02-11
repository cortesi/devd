
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

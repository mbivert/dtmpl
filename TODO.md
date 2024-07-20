# Automated tests @tests
This has been roughly tested by hand, but we'll want specific,
per-function tests (some of them are bug-prone).

# Documentation @documentation @man-page
In particular, complete the man page and describe the template
usage conventions.

# Templatized filenames @template-fn
**<u>Note:</u>** [``old/dtmpl.go``][gh-mb-dtmpl-old] is a rough,
uncleaned, older implementation which still has that feature.

I'm not convinced this is actually a good thing to implement (hence
why it's been removed) but in case this comes useful again later:

The idea is that if you have a path "pages/tags/{{.tags}}.html.tmpl", were
"tags" is a database entry, we could automatically replace this file
from the input directory by a set of files matching the registered
db.tags entry:
	- if it's a scalar, one output file, by substituing the value
	- if it's an array, one output file per array entry

If it's a map, it's not so clear what we should do.

Multiple placeholders could be used: "pages/{{.foo}}/{{.bar}}.html

Furthermore, we'll often want to have access to the substitued value
say within the template, associated to the file. It's cumbersome to
have to look in the path to get it, but it could be injected in the
files header (which would be bootstraped from the original file).

Now this injection could be confusing, say adding an entry "tags"
could mess things up, but we could use an extra program to generate
a special list "enum-tag", before running ``dtmpl``, and use
this as a new field.

[gh-mb-dtmpl-old]: https://github.com/mbivert/dtmpl/blob/master/old/dtmpl.go
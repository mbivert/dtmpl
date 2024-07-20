# Introduction
``dtmpl(1)`` processes an input directory to an output directory,
processing files suffixed by ``.tmpl`` with ``go(1)``'s
[``text/template``][go-text/template]. *All* the files which
aren't suffixed by ``.tmpl`` are copied from the input directory
to the output directory.

The input directory may furthermore contain:

  - a ``db.json`` file and/or a ``db/`` directory: both of them
  describe a JSON-encoded database which is made available to
  executed templates;
  - a ``templates/`` directory, which contains a bunch of "utility"
  templates, which can call each other, and can be called from the
  ``.tmpl`` files.

You can see it being used as a primitive static site generator
[here][gh-mb-bargue].

[go-text/template]: https://pkg.go.dev/text/template
[gh-mb-bargue]: https://github.com/mbivert/bargue/

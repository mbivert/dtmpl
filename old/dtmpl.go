package main

// TODO: make is to that file ids are their path [relative to ind]

// dtpml(1) - directory templates
//
// Compile an input directory to an output directory by
// identifying special files and special file/directory names,
// and templatizing them using go's text/template.
//
// This can typically be used to implement a static site generator,
// at least partially.
//
// The input directory can contain a data/ directory, containing
// JSON files, which forms a global database (XXX for convenience,
// do we allow nested files and load everything as a hash, or do
// we keep a bare data.json file, and allow this feature to be
// implemented by an extra call to dp with some specialized program?),
// which is made available to the templates.
//
// By default, files from the input directory are systematically
// copied to the output directory, to the following exceptions.
//
// First, file suffixed by base.tmpl are interpreted as templates, processed
// using text/template, and copied to the output directory with the
// .tmpl suffix removed.
//
// Second, if for such a .tmpl file there's a corresponding
// .tmpl.json, it is loaded in the database so as to provide additional
// variables, and eventually temporarily overides some, and then the
// .tmpl file is processed as previously described.
//
// Note that a special database variable "filename" always points to
// the path of the currently processed file (needed?)
//
// Third, filenames of the starting with a string of the form "{{%name%}}"
// are first processed as such, assuming name points to some (eventually nested)
// database entry:
//
//	- if the entry is a  scalar, then the file is renamed accordingly;
//	- if the entry is an array,  then one file per entry is created;
//	- if the entry is a  hash,   then one file per key   is created.
//
// In all cases, only {{%name%}} is substituted. In particular, if the file is a
// .tpml, or a .tmpl.json, is is processed as described earlier, after renaming.
//
// If the file is a directory, then the renaming/creation still happens,
// the content of the directory is duplicated as many times as needed, and
// the walk continues in each such directory.
//
// (XXX this might be clumsy, and is typically one case where calling
// dp to sort those things before hand might be cleaner. Perhaps having dp
// being available as a command and as a Go module could help)

// Process for a dp lib would be similar getin(); runcmd(); setout().
// File serialization could now be a hash path => content + metas

// Sample use case: /{{%.tags%}}/index.html.tmpl
//	dp transforms
//		/{{%.tags%}}/index.html.tmpl
//	to:
//		/tag0/index.html.tmpl
//		/tag1/index.html.tmpl
//		...

// A problem with the previous approach is that we can't programatically
// generate the index.html.tmpl from the data related to .tags: for example
// in the case of bargue -- w.r.t. what's done in bargue-pp.go -- we need
// to generate say templates like {{%.pages.$name.$lang.url%}}/index.html.tmpl
// (fantasist syntax), only for plates, that will be parametrized by the
// $name, $lang and also the url:
/*
{{< header "fr" >}}

{{< plate "plate_I-01_eyes" "fr" >}}

{{< maybeparsefn "/fr/planche_I-01_yeux//index.html.content" >}}

{{< footer "fr" >}}
*/
// We could adjust it as:
/*
{{< header "fr" >}}

{{< plate "plate_I-01_eyes" "fr" >}}

{{< maybeparsefn (printf "%s/%s" (index .db.pages "plates_I-01_eyes" "fr" "url") "index.html.content") >}}

{{< footer "fr" >}}
*/
// Or even hide all in a template, which would at least reduce
// the boilerplate
/*
{{< platepage "plate_I-01_eyes" "fr" >}}
*/
// Now, we could be smart, and from the previous template filename syntax
// ({{%.pages.$id.$lang.url%}}/index.html.tmpl) prepend to the default .html.tmpl:
//
//	{{< platepage $id $lang >}}
//
// A list of variables, whose value would depend from how we're ranging on
// variables in the template filename:
//	{{< $id   := ... >}}
//	{{< $lang := ... >}}
// Hence the final generated templates would be things like:
/*
{{< $id   := ... >}}
{{< $lang := ... >}}
{{< platepage $id $lang >}}
*/
//
// We would still have one issue: we'd be generating this for all pages, not
// just for plates. Unless we had yet another indirection layer in the template
// (eg. mkpage $id $lang which would call mkplatepage, mkgrouppage, etc.)
//	-> This could be solve by using directories / different templates for each
//	type of pages too, e.g.
//	/plate/foo
//	/group/bar
//	/fr/planche/foo

/*
	Another option would be to have still a template filename such as
		{{{% .pages.$id.$lang.url %}}}/index.html.tmpl
	With index.html.tmpl containing
		{{< platepage {{{ $id }}} {{{ $lang }}} >}}
	Which, upon page creation, would be templatized a first time with
	the relevant values for $id and $lang, e.g. we'll generate a file
		/planche_I,01_yeux/index.html.tmpl
	containing:
		{{< platepage "plate_I,01_eyes" "fr" >}}
*/

// How about compiling multi-layered hashes, e.g.
//	material : {
//		pencil : {
//			"hb-staedler" : ...,
//		}
//	}
//
// Imagine: //{{%.material%}}/{{%.%}}/index.html.tmpl
// Or /{{%.foo%}}{{%.bar%}}/index.html.tmpl

import (
	"encoding/json"
	"errors"
	"fmt"
//	"html"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
//	"runtime/pprof"
)

var ind, outd string

var tmplExt = ".tmpl"

// TODO: CLI argument, use a "ids" default?
var idsKey  = "pages"

type FNs  map[string]any

type DB map[string]any

var tmpls *template.Template
var db DB

func splitPath(path string) []string {
	return strings.Split(path, string(os.PathSeparator))
}

func addFn(ind string, fns FNs, path string) FNs {
	xs := splitPath(path)
	var p FNs
	p = fns
	for i, x := range xs {
		if i == len(xs)-1 {
			// TODO: don't do this here, but when reading
			// the files later on
			p[x] = filepath.Join(ind, path)
		} else {
			if _, ok := p[x]; !ok {
				p[x] = make(FNs, 1)
			}
			q, ok := p[x].(FNs)
			if !ok {
				q = make(FNs, 1)
				p[x] = q
			}
			p = q

		}
	}
	return fns
}

func loadFNs(ind string) (FNs, error) {
	fns := make(FNs, 1)
	err := filepath.Walk(ind, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// useless
		if path == ind {
			return nil
		}

		// ind is filepath.Clean()'d ':/^func init\('
		path = strings.TrimPrefix(path, ind+string(os.PathSeparator))

		fns = addFn(ind, fns, path)

		return nil
	})

//	fmt.Fprint(os.Stderr, fns)

	return fns, err
}

// TODO: nested db directory support?
func loadDB(ind string) (DB, error) {
	var db DB
	raw, err := os.ReadFile(filepath.Join(ind, "db.json"))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &db)
	return db, err
}

func getNames(s string, db DB /*, pdot */) []string {
	// Generous enough
	re := regexp.MustCompile(`{{[^}]*}}`)

	xs := re.FindAllStringIndex(s, -1)

	if len(xs) == 0 {
		return []string{s}
	}

	// TODO: manage multiple {{.}} at once? (recursive then)
	x := xs[0]
	y := strings.TrimSpace(s[x[0]+2:x[1]-2])
	if  y[0] == '.' {
		y = y[1:]
	}
	v, ok := db[y]
	if !ok {
		return []string{s}
	}

	if w, ok := v.(string); ok {
		return []string{s[0:x[0]]+w+s[x[1]:len(s)]}
	}

	if ws, ok := v.([]any); ok {
		zs := make([]string, 0)
		for _, w := range ws {
			t, _ := w.(string) // XXX
			zs = append(zs, s[0:x[0]]+t+s[x[1]:len(s)])
		}
		return zs
	}

	if ws, ok := v.(map[string]any); ok {
		zs := make([]string, 0)
		for w, _ := range ws {
			zs = append(zs, s[0:x[0]]+w+s[x[1]:len(s)])
		}
		return zs
	}

	panic("nein, "+s)

	return []string{}
}

func tmplfns(fns FNs, db DB, pdot []string) (FNs, error) {
	tfns := make(FNs, 1)
	var err error

	for k, v := range fns {
		for _, n := range getNames(k, db /* pdot */) {
			if w, ok := v.(FNs); ok {
				if tfns[n], err = tmplfns(w, db , append(pdot, k)); err != nil {
					return tfns, err
				}
			} else {
				tfns[n] = v
			}
		}
	}

	return tfns, nil
}

func getKeys[T any] (xs map[string]T) []string {
	ks := make([]string, 0, len(xs))
	for k := range xs {
		ks = append(ks, k)
	}
	return ks
}

func copyFile(from, to string, perm os.FileMode) error {
	xs, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, xs, perm)
}

func addToPATH(p string) string {
	path := os.Getenv("PATH")
	return path+":"+p
}

func loadTmpls(ind string, db DB) *template.Template {
	var tmpls *template.Template
	tmpls = template.Must(template.New("").Funcs(template.FuncMap{
		"append" :  func(xs []any, ys []any) []any {
			return append(xs, ys...)
		},
		"join" : func(xs []any, d string) string {
			ys := make([]string, len(xs))
			for i, x := range xs {
				ys[i] = fmt.Sprint(x)
			}
			return strings.Join(ys, d)
		},
		"contains" : func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"warn" : func(s string) string {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", s)
			return ""
		},
		"isURL" : func(s string) bool {
			_, err := url.ParseRequestURI(s)
			return err == nil
		},
		// NOTE: this is the only [default] template function
		// explicitely depending on .db.id, but the field is
		// used in the default templates from the template/
		// directory
		"get" : func(xs ...string) (any, error) {
			id, _ := db["id"].(string)
			x, err := deepGet(db, append([]string{idsKey, id}, xs...))
			if err != nil {
				fmt.Fprintf(os.Stderr, "get(id=%s, xs=%v): err=%s\n", id, xs, err)
			}
			return x, err
		},
		"add" : func(a, b any) (int, error) {
			an, ok := a.(int)
			if !ok {
				as, ok := a.(string)
				if !ok {
					return 0, fmt.Errorf("add: '%v' not an integer?", a)
				}
				var err error
				an, err = strconv.Atoi(as)
				if err != nil {
					return 0, err
				}
			}

			bn, ok := b.(int)
			if !ok {
				bs, ok := b.(string)
				if !ok {
					return 0, fmt.Errorf("add: '%v' not an integer?", b)
				}
				var err error
				bn, err = strconv.Atoi(bs)
				if err != nil {
					return 0, err
				}
			}

			return an+bn, nil
		},
		"sarr" : func(xs ...string) []string {
			return xs
		},
		"arr" : func(xs ...any) []any {
			return xs
		},
		"wrap" : func(xs ...any) any {
			return map[string]any {
				"db"   : db,
				"args" : xs,
			}
		},
		"parse" : func(ts string) (string, error) {
			t, err := template.Must(tmpls.Clone()).Parse(ts)
			if err != nil {
				return "", err
			}

			var s strings.Builder
			err = t.Execute(&s, map[string]any{
				"db" : db,
			})
			return s.String(), err
		},
		"now" : func() time.Time {
			return time.Now()
		},
		"exists" : func(path string) (bool, error) {
//			return false, nil
			if _, err := os.Stat(filepath.Join(ind, path)); err == nil {
				return true, nil
			} else if errors.Is(err, os.ErrNotExist) {
				return false, nil
			} else {
				return false, err
			}

			return false, nil
		},
		"include" : func(path string) (string, error) {
			path = filepath.Join(ind, path)
//			fmt.Fprintf(os.Stderr, "including %s\n", path)
			xs, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: inclusion failed '%s', %s\n", path, err)
			}
			return string(xs), err
		},
		"run" : func(this *template.Template, cmd []string, x string, targs ...any) (string, error) {
			t := template.Must(this.Clone())

			if len(cmd) < 1 {
				return "", fmt.Errorf("No command?")
			}

//			fmt.Fprintf(os.Stderr, "running: %v, %s, %v\n", cmd, x, targs)

			args := cmd[1:]
			if x != "" {
				// NOTE: we use a file instead of a pipe to avoid having
				// to deal with process synchronization
				fn := filepath.Join("/tmp", x)
				f, err := os.OpenFile(fn, os.O_RDWR|os.O_CREATE, 0644)
				if err != nil {
					return "", err
				}

				err = t.ExecuteTemplate(f, x, map[string]any{
					"args" : targs,
					"this" : t,
				})
				f.Close()
				if err != nil {
					return "", err
				}
				args = append(cmd[1:], fn)
			}

			var s strings.Builder
			com := exec.Command(cmd[0], args...)
			com.Dir    = ind
			com.Stdout = &s
			com.Stderr = &s

			if err := com.Run(); err != nil {
				return "", err
			}

			return s.String(), nil
		},
		"datefmt" : func(ds, inf, outf string) (string, error) {
			if inf == "" {
				inf = time.RFC3339
			}
			if outf == "" {
				outf = inf
			}
			d, err := time.Parse(inf, ds)
			if err != nil {
				return "", err
			}
			return d.Format(outf), nil
		},
	}).ParseGlob(filepath.Join(ind, "templates/*")))

	// Make functions out of the default templates from the
	// templates/ directory.
	for _, x := range tmpls.Templates() {
		n := strings.TrimSuffix(x.Name(), tmplExt)
		// beware of the race...
		m := x.Name()
		tmpls.Funcs(template.FuncMap{
			n : func(ys ...any) (string, error) {
				var s strings.Builder
				err := tmpls.ExecuteTemplate(&s, m, map[string]any{
					"args" : ys,
					"db"   : db,
				})
				return s.String(), err
			},
		})
	}

	return tmpls
}

// TODO: ~same code in mkdb.go
func deepGet(db DB, xs []string) (any, error) {
	var p map[string]any
	p = db
	for n, x := range xs {
		if n == len(xs)-1 {
			return p[x], nil
		}
		q, ok := p[x]
		if !ok {
			break
		}
		r, ok := q.(map[string]any)
		if !ok {
			// TODO error
			panic("nein")
		}
			p = r
	}

//	fmt.Fprintf(os.Stderr, "%v\n%v", db, xs)
	return nil, fmt.Errorf("not found")
}

func getId(ind, path string) string {
	// TODO/XXX: do it somewhere else, only once
	ind = filepath.Clean(ind)+string(os.PathSeparator)
//	return strings.TrimPrefix(path, ind)
	return strings.TrimSuffix(strings.TrimPrefix(path, ind), tmplExt)
}

func getExt(fn string) string {
	x := ""
	for {
		y := filepath.Ext(fn)
		if y == "" {
			break
		}
		x = y+x
		fn = strings.TrimSuffix(fn, y)
	}
	return x
}

func tmplFile(from, to string, db DB) error {
	to = strings.TrimSuffix(to, tmplExt)

	t, err := template.Must(tmpls.Clone()).Delims("{{<", ">}}").ParseFiles(from)

	if err != nil {
		return err
	}

	fh, err := os.OpenFile(to, os.O_RDWR|os.O_CREATE, 0644)
	defer fh.Close()
	if err != nil {
		return err
	}

	id  := getId(ind, from)
	ext := getExt(id)

	// NOTE: it's important to understand that we rely on db being a pointer
	// shared by all the templates.
	//
	// This means .db.id & cie will always point to the currently processed
	// article even in "base" templates from the template/ directory.
	//
	// This implies that we can't process templates in parallel.
	db["id"]   = id
	db["ext"]  = ext
	db["path"] = splitPath(to) // XXX/TODO: useless, as we can't index it arbitrarily?
	db["base"] = filepath.Base(to) // XXX/TODO: still contains an extensions ($tag.md)

	// NOTE: all templates (see ':/^func loadTmpls\(', especially the
	// wrap() and parse() template functions) are executed with a hash
	// pipeline. Said hash contains at least a .db, which as stated earlier,
	// not only contains the database but some fields pertaining to the
	// current file. Additionally, a .args entry may contain template-specific
	// arguments.
	//
	// TODO: store db.id & cie in a new cur field.
	targs := map[string]any{
		"db"   : db,
		"this" : t,
	}

	// NOTE: originally, we would systematically execute a
	// pair of header/footer template, which could be changed
	// using special metas. However, markdown doesn't play well
	// with a subsequent HTML input.
	//
	// Going the other way around, meaning, first calling markdown
	// then the templates still brought some complications.
	//
	// Hence for now, header/footer are generated separately,
	// created by mkdb and joined in tohtml to create the final
	// files.
	//
	// TODO: we may still want to allow such a regular header/footer
	// setting for some files.
	return t.ExecuteTemplate(fh, filepath.Base(from), targs)
}

// TODO : naming case
func tmplfiles(outd string, tfns FNs, db DB, p []string) error {
	for k, v := range tfns {
		fn := filepath.Join((append(p, k))...)

		if w, ok := v.(FNs); ok {
			if err := os.MkdirAll(fn, os.ModePerm); err != nil {
				return err
			}
			if err := tmplfiles(outd, w, db, append(p, k)); err != nil {
				return err
			}
		} else if w, ok := v.(string); ok {
			var err error
			if filepath.Ext(fn) == tmplExt {
//				fmt.Println("Processing", w, "to", fn)
				err = tmplFile(w, fn, db)
			} else {
//				fmt.Println("Copying", w, "to", fn)
				err = copyFile(w, fn, 0644)
			}
			if err != nil {
				return err
			}
		} else {
			panic("nein.")
		}
	}

	return nil
}

func dtmpl(ind, outd string) error {
	// 1. Load input directory filenames
	fns, err := loadFNs(ind)
	if err != nil {
		return err
	}
//	fmt.Println("fns", fns)

	if x, ok := fns[ind].(FNs); ok {
		fns = x
	}

	// 2. Generate templated filenames
	tfns, err := tmplfns(fns, db, []string{})
	if err != nil {
		return err
	}

//	fmt.Println("tfns", tfns)

	// 3. Generate file contents
	return tmplfiles(outd, tfns, db, []string{outd})
}

func help(n int) {
	argv0 := path.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "%s <input/> <output/>\n", argv0)
	os.Exit(n)
}

func fails(err error) {
	argv0 := path.Base(os.Args[0])
	log.Fatal(argv0, ": ", err)
}

func init() {
	fmt.Fprintf(os.Stderr, "dtmpl begin: %s\n", time.Now())
	var err error

	if len(os.Args) != 3 {
		help(1)
	}
	// TODO: dp.go, etc.
	ind, outd = filepath.Clean(os.Args[1]), filepath.Clean(os.Args[2])

	os.Setenv("PATH", addToPATH(filepath.Join(ind, "bin")))
//	fmt.Fprintf(os.Stderr, os.Getenv("PATH")+"\n")

	// TODO: systematically trim before / have an option to?
	if err = os.MkdirAll(outd, os.ModePerm); err != nil {
		fails(err)
	}

	// Load input directory database
	db, err = loadDB(ind)
	if err != nil {
		fails(err)
	}

//	a, _ := json.MarshalIndent(db, "", "    ")
//	fmt.Fprintf(os.Stderr, "db: %s\n", string(a))

	tmpls = loadTmpls(ind, db)
}

func main() {
/*
	{
        f, err := os.Create("/tmp/dtmpl.pprof")
        if err != nil {
            log.Fatal(err)
        }
        pprof.StartCPUProfile(f)
        defer pprof.StopCPUProfile()
    }
*/

	// TODO: inline dtmpl
	if err := dtmpl(ind, outd); err != nil {
		fails(err)
	}
	fmt.Fprintf(os.Stderr, "dtmpl end: %s\n", time.Now())
}

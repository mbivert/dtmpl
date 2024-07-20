package main

// dtpml(1) - Deep/directory template
//
// Compile an input directory to an output directory, by
// templatizing some input files via go(1)'s text/template.
//
// This can typically be used e.g. to implement a basic static
// site generator.
//
// The input directory may contain:
//
//	- a db/ directory or/and a db.json file. A database is constructed
//	and from all those files, and can be accessed from the templates.
//
//	-a templates/ directory, which contains templates which can be
//	called from elsewhere.
//
// Everything else is copied to the output directory. Files suffixed
// with ".tmpl" (default value) are first processed with text/template,
// the result is then sent to the output directory; the output filename
// is stripped from its .tmpl suffix.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// input/output directories
var ind, outd string

// TODO: most of those should be CLI params
var tmplExt = ".tmpl"
var jsonExt = ".json"

var dbDir = "db"
var dbFn  = "db.json"

var dropDB = true

var tmplsDir = "templates"

type FNs map[string]any

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

		// useless entry (but should be essentially benign because
		// of the following TrimPrefix).
		if path == ind {
			return nil
		}

		// Those have been loaded separately, and we
		// don't want to bring them to the output directory
		if dropDB && path == filepath.Join(ind, dbFn) {
			return nil
		}
		if dropDB && strings.HasPrefix(path, filepath.Join(ind, dbDir)) {
			return nil
		}

		// ind is filepath.Clean()'d ':/^func init\('
		path = strings.TrimPrefix(path, ind+string(os.PathSeparator))

		fns = addFn(ind, fns, path)

		return nil
	})

	return fns, err
}

// TODO: path vs. fn naming convention
func doParseDBFile(path string, to *any) error {
	e := filepath.Ext(path)

	bs, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if e == jsonExt {
		if err := json.Unmarshal(bs, &to); err != nil {
			return fmt.Errorf("%s: %s", path, err)
		}
	} else {
		return fmt.Errorf("Unknown data/* format "+e)
	}

	return nil
}

func storeDBFile(ind, path string, db DB, y any) error {
	// NOTE: extra os.PathSeparator is requires, for filepath.Join
	// would trim it (even with a "db/"), and splitPath would return
	// an array starting with an empty string.
	xs := splitPath(
		strings.TrimSuffix(
			strings.TrimPrefix(path, filepath.Join(ind, dbDir)+string(os.PathSeparator)),
			filepath.Ext(path),
		),
	)
//	fmt.Fprintf(os.Stderr, path, xs)

	var p map[string]any
	p = db
	for n, x := range xs {
		if n == len(xs)-1 {
			// merge y and p[x]; for now, this is
			// only because we're storing internal
			// URLs alongside external URLs, so
			// as to have them all managed by the same
			// template.
			// XXX/TODO errors & cie
			if z, ok := y.(map[string]any); ok {
				if q, ok := p[x].(map[string]any); ok {
					for k, v := range z {
						q[k] = v
					}
					continue
				}
			}
			p[x] = y
			break
		}
		q, ok := p[x]
		if !ok {
			q := make(map[string]any)
			p[x] = q
			p = q
		} else {
			r, ok := q.(map[string]any)
			if !ok {
				// TODO: better error management
				panic("x__x")
			}
			p = r
		}
	}

	return nil
}

// TODO: manage deeper nesting / no-nesting
func parseDBFile(ind, path string, db DB) error {
	var y any
	if err := doParseDBFile(path, &y); err != nil {
		return err
	}

	return storeDBFile(ind, path, db, y)
}

func loadDBDir(ind string, db DB) (DB, error) {
	dbd := filepath.Join(ind, dbDir)
	err := filepath.Walk(dbd, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		return parseDBFile(ind, path, db)
	})

	return db, err
}

func loadDB(ind string) (DB, error) {
	var db DB
	fn := filepath.Join(ind, dbFn)
	raw, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &db)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", fn, err)
	}
	return loadDBDir(ind, db)
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
		"append" :  func(xs []any, ys []any) []any {
			return append(xs, ys...)
		},
		"arr" : func(xs ...any) []any {
			return xs
		},
		"contains" : func(s, substr string) bool {
			return strings.Contains(s, substr)
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
		"exists" : func(path string) (bool, error) {
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
			xs, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: inclusion failed '%s', %s\n", path, err)
			}
			return string(xs), err
		},
		"isURL" : func(s string) bool {
			_, err := url.ParseRequestURI(s)
			return err == nil
		},
		"join" : func(xs []any, d string) string {
			ys := make([]string, len(xs))
			for i, x := range xs {
				ys[i] = fmt.Sprint(x)
			}
			return strings.Join(ys, d)
		},
		"now" : func() time.Time {
			return time.Now()
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
		// Some of that is more thoroughly documented here:
		//	https://tales.mbivert.com/on-piping-go-templates-to-shell/
		"run" : func(this *template.Template, cmd []string, x string, targs ...any) (string, error) {
			t := template.Must(this.Clone())

			if len(cmd) < 1 {
				return "", fmt.Errorf("No command?")
			}

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
		"sarr" : func(xs ...string) []string {
			return xs
		},
		"warn" : func(s string) string {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", s)
			return ""
		},
		"wrap" : func(xs ...any) any {
			return map[string]any {
				"db"   : db,
				"args" : xs,
			}
		},
	}).ParseGlob(filepath.Join(ind, tmplsDir+"/*")))

	// Make functions out of the default templates from the
	// templates/ directory. For more, see
	//	https://tales.mbivert.com/on-a-pool-of-go-templates/
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
			if _, ok := q.(string); ok {
				return nil, fmt.Errorf("Can't go further in DB (%s)",
					strings.Join(xs[:n], " -> "))
			}
			// Internal error (~assert)
			panic("O__o")
		}
		p = r
	}

	return nil, fmt.Errorf("not found")
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

	// NOTE: all templates (see ':/^func loadTmpls\(', especially the
	// wrap() and parse() template functions) are executed with a hash
	// pipeline. Said hash contains at least a .db.
	//
	// We're trying to make this interface more "uniform".
	return t.ExecuteTemplate(fh, filepath.Base(from), map[string]any{
		"db"   : db,
		"this" : t, // seems it's still used for run()
	})
}

func tmplFiles(outd string, tfns FNs, db DB, p []string) error {
	for k, v := range tfns {
		fn := filepath.Join((append(p, k))...)

		if w, ok := v.(FNs); ok {
			if err := os.MkdirAll(fn, os.ModePerm); err != nil {
				return err
			}
			if err := tmplFiles(outd, w, db, append(p, k)); err != nil {
				return err
			}
		} else if w, ok := v.(string); ok {
			var err error
			if filepath.Ext(fn) == tmplExt {
				err = tmplFile(w, fn, db)
			} else {
				err = copyFile(w, fn, 0644)
			}
			if err != nil {
				return err
			}
		} else {
			panic("O_o")
		}
	}

	return nil
}

func dtmpl(ind, outd string) error {
	// Load input directory filenames
	fns, err := loadFNs(ind)
	if err != nil {
		return err
	}

	// filenames in fns are relative to ind
	// (~assert)
	if _, ok := fns[ind].(FNs); ok {
		panic("O__O")
	}

	a, _ := json.MarshalIndent(db, "", "    ")
	fmt.Fprintf(os.Stderr, "db = %s\n", string(a))

	b, _ := json.MarshalIndent(fns, "", "    ")
	fmt.Fprintf(os.Stderr, "fns = %s\n", string(b))

	// Generate file contents
	return tmplFiles(outd, fns, db, []string{outd})
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
	var err error

	// TODO: have those+tmplsDir be not relative to ind but paths
	// to exact files instead (too magic)
	flag.StringVar(&dbFn,  "dbFn",  dbFn,  "Default path to db.json (relative to ind)")
	flag.StringVar(&dbDir, "dbDir", dbDir, "Default path to db/ (relative to ind)")

	flag.StringVar(&tmplExt, "tmplExt", tmplExt, "Default template files extension")
	flag.StringVar(&tmplsDir, "tmplsDir", tmplsDir, "Default path to template/ (relative to ind)")

	flag.BoolVar(&dropDB, "dropDB", dropDB, "By default, don't output the input db-related files")

	flag.Parse()

	if len(flag.Args()) != 2 {
		help(1)
	}

	ind, outd = filepath.Clean(flag.Args()[0]), filepath.Clean(flag.Args()[1])

	// NOTE: As RemoveAll() is preferable, I haven't dig deeper, but
	// without the RemoveAll(), there's sometimes garbage by the end
	// of the generated files.
	if err = os.RemoveAll(outd); err != nil {
		fails(err)
	}
	if err = os.MkdirAll(outd, os.ModePerm); err != nil {
		fails(err)
	}

	// Load input directory's database
	db, err = loadDB(ind)
	if err != nil {
		fails(err)
	}

	// Load input directory's templates/ directory
	tmpls = loadTmpls(ind, db)
}

func main() {
	if err := dtmpl(ind, outd); err != nil {
		fails(err)
	}
}

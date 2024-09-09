# dtmpl(1) - deep/directory templater

    dtmpl(1)                     General Commands Manual                   dtmpl(1)
    
    NAME
           dtmpl — deep/directory templater 1.0
    
    SYNOPSIS
           dtmpl [-h]
           dtmpl  [-f 'db.json'] [-d 'db/'] [-e '.tmpl'] [-t 'templates/'] <input/>
                 <output/>
    
    DESCRIPTION
           dtmpl processes  an  input  directory  input/  to  an  output  directory
           output/, processing files suffixed by .tmpl (can be altered with -e) via
           go(1)  's  text/template  (see  https://pkg.go.dev/text/template).   All
           files which aren't suffixed by .tmpl are copied from the input directory
           to the output directory.
    
           The input directory may furthermore contain:
    
           1.   A db.json file and/or a db/ directory:  both  of  them  describe  a
                (deep)  JSON-encoded  database, which is made available to the exe‐
                cuted templates (input pipeline is a map containaing a db field);
    
           2.   a templates/ directory, which contains a bunch  of  "utility"  tem‐
                plate  files, which can call each other, and can be called from the
                .tmpl -suffixed files.
    
           By default, those files aren't preserved to the output directory, unless
           -k is provided.
    
    TEMPLATE CONVENTIONS
           All the template files in  the  templates/  directory  are  provided  as
           template  functions,  meaning, a template file templates/foo is callable
           as
    
                     {{< foo arg0 arg1 >}}
           Or
    
                     {{< template "foo" wrap arg0 arg1 >}}
    
           Where ‘wrap’ is a template function, described in the next section.
    
           The point of ‘wrap’ or of making the templates  "callable"  is  to  have
           them all share a similar interface: all the .tmpl suffixed files as well
           as  all  the templates from the templates/ directory are provided with a
           "map-pipeline" containing:
    
           1.   A ‘db’ entry, which contains the parsed db.json and db/
    
           2.   Eventually for the  templates  from  the  templates/  directory,  a
                ‘args’  entry,  which contains an array with all the arguments pro‐
                vided to the template.
    
           For example, the ‘wrap’ function mentioned previously is implemented  as
           follow:
    
                     "wrap" : func(xs ...any) any {
                         return map[string]any {
                                 "db"   : db,
                                 "args" : xs,
                         },
                     },
    
           The    technique    is    described    in    greater    details    here:
           https://tales.mbivert.com/on-a-pool-of-go-templates/
    
    TEMPLATE FUNCTIONS
           For convenience, a few base functions are provided for use in  the  tem‐
           plates. TODO
    
    EXAMPLE
           Static    site    generator    with    a   bunch   of   extra-templates:
           https://github.com/mbivert/bargue
    
    SEE ALSO
           dp(1) (see https://github.com/mbivert/dp)
    
    dtmpl 1.0                             2024                             dtmpl(1)

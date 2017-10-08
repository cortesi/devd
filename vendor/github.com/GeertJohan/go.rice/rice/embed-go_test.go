package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type registeredDir struct {
	Filename   string
	ModTime    int
	ChildFiles []*registeredFile
	ChildDirs  []*registeredDir
}

type registeredFile struct {
	Filename string
	ModTime  int
	Content  string
}

type registeredBox struct {
	Name string
	Time int
	// key is path
	Dirs map[string]*registeredDir
	// key is path
	Files map[string]*registeredFile
}

// isSimpleSelector returns true if expr is pkgName.ident
func isSimpleSelector(pkgName, ident string, expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if pkgIdent, ok := sel.X.(*ast.Ident); ok && pkgIdent.Name == pkgName && sel.Sel != nil && sel.Sel.Name == ident {
			return true
		}
	}
	return false
}

func isIdent(ident string, expr ast.Expr) bool {
	if expr, ok := expr.(*ast.Ident); ok && expr.Name == ident {
		return true
	}
	return false
}

func getIdentName(expr ast.Expr) (string, bool) {
	if expr, ok := expr.(*ast.Ident); ok {
		return expr.Name, true
	}
	return "", false
}

func getKey(expr *ast.KeyValueExpr) string {
	if ident, ok := expr.Key.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// parseModTime parses a time.Unix call, and returns the unix time.
func parseModTime(expr ast.Expr) (int, error) {
	if expr, ok := expr.(*ast.CallExpr); ok {
		if !isSimpleSelector("time", "Unix", expr.Fun) {
			return 0, fmt.Errorf("ModTime is not time.Unix: %#v", expr.Fun)
		}
		if len(expr.Args) == 0 {
			return 0, fmt.Errorf("not enough args to time.Unix")
		}
		arg0 := expr.Args[0]
		if lit, ok := arg0.(*ast.BasicLit); ok && lit.Kind == token.INT {
			return strconv.Atoi(lit.Value)
		}
	}
	return 0, fmt.Errorf("not time.Unix: %#v", expr)
}

func parseString(expr ast.Expr) (string, error) {
	if expr, ok := expr.(*ast.CallExpr); ok && isIdent("string", expr.Fun) && len(expr.Args) == 1 {
		return parseString(expr.Args[0])
	}
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		return strconv.Unquote(lit.Value)
	}
	return "", fmt.Errorf("not string: %#v", expr)
}

// parseDir parses an embedded.EmbeddedDir literal.
// It can be either a variable name or a composite literal.
// Returns nil if the literal is not embedded.EmbeddedDir.
func parseDir(expr ast.Expr, dirs map[string]*registeredDir, files map[string]*registeredFile) (*registeredDir, []error) {

	if varName, ok := getIdentName(expr); ok {
		dir, ok := dirs[varName]
		if !ok {
			return nil, []error{fmt.Errorf("unknown variable %v", varName)}
		}
		return dir, nil
	}

	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, []error{fmt.Errorf("dir is not a composite literal: %#v", expr)}
	}

	var errors []error
	if !isSimpleSelector("embedded", "EmbeddedDir", lit.Type) {
		return nil, nil
	}
	ret := &registeredDir{}
	for _, el := range lit.Elts {
		if el, ok := el.(*ast.KeyValueExpr); ok {
			key := getKey(el)
			if key == "" {
				continue
			}
			switch key {
			case "DirModTime":
				var err error
				ret.ModTime, err = parseModTime(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("DirModTime %s", err))
				}
			case "Filename":
				var err error
				ret.Filename, err = parseString(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("Filename %s", err))
				}
			case "ChildDirs":
				var errors2 []error
				ret.ChildDirs, errors2 = parseDirsSlice(el.Value, dirs, files)
				errors = append(errors, errors2...)
			case "ChildFiles":
				var errors2 []error
				ret.ChildFiles, errors2 = parseFilesSlice(el.Value, files)
				errors = append(errors, errors2...)
			default:
				errors = append(errors, fmt.Errorf("Unknown field: %v: %#v", key, el.Value))
			}
		}
	}
	return ret, errors
}

// parseFile parses an embedded.EmbeddedFile literal.
// It can be either a variable name or a composite literal.
// Returns nil if the literal is not embedded.EmbeddedFile.
func parseFile(expr ast.Expr, files map[string]*registeredFile) (*registeredFile, []error) {
	if varName, ok := getIdentName(expr); ok {
		file, ok := files[varName]
		if !ok {
			return nil, []error{fmt.Errorf("unknown variable %v", varName)}
		}
		return file, nil
	}

	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, []error{fmt.Errorf("file is not a composite literal: %#v", expr)}
	}

	var errors []error
	if !isSimpleSelector("embedded", "EmbeddedFile", lit.Type) {
		return nil, nil
	}
	ret := &registeredFile{}
	for _, el := range lit.Elts {
		if el, ok := el.(*ast.KeyValueExpr); ok {
			key := getKey(el)
			if key == "" {
				continue
			}
			switch key {
			case "FileModTime":
				var err error
				ret.ModTime, err = parseModTime(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("DirModTime %s", err))
				}
			case "Filename":
				var err error
				ret.Filename, err = parseString(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("Filename %s", err))
				}
			case "Content":
				var err error
				ret.Content, err = parseString(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("Content %s", err))
				}
			default:
				errors = append(errors, fmt.Errorf("Unknown field: %v: %#v", key, el.Value))
			}
		}
	}
	return ret, errors
}

func parseRegistration(lit *ast.CompositeLit, dirs map[string]*registeredDir, files map[string]*registeredFile) (*registeredBox, []error) {
	var errors []error
	if !isSimpleSelector("embedded", "EmbeddedBox", lit.Type) {
		return nil, nil
	}
	ret := &registeredBox{
		Dirs:  make(map[string]*registeredDir),
		Files: make(map[string]*registeredFile),
	}
	for _, el := range lit.Elts {
		if el, ok := el.(*ast.KeyValueExpr); ok {
			key := getKey(el)
			if key == "" {
				continue
			}
			switch key {
			case "Time":
				var err error
				ret.Time, err = parseModTime(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("Time %s", err))
				}
			case "Name":
				var err error
				ret.Name, err = parseString(el.Value)
				if err != nil {
					errors = append(errors, fmt.Errorf("Name %s", err))
				}
			case "Dirs":
				var errors2 []error
				ret.Dirs, errors2 = parseDirsMap(el.Value, dirs, files)
				errors = append(errors, errors2...)
			case "Files":
				var errors2 []error
				ret.Files, errors2 = parseFilesMap(el.Value, files)
				errors = append(errors, errors2...)
			default:
				errors = append(errors, fmt.Errorf("Unknown field: %v: %#v", key, el.Value))
			}
		}
	}
	return ret, errors
}

func parseDirsSlice(expr ast.Expr, dirs map[string]*registeredDir, files map[string]*registeredFile) (childDirs []*registeredDir, errors []error) {
	valid := false
	lit, ok := expr.(*ast.CompositeLit)
	if ok {
		if arrType, ok := lit.Type.(*ast.ArrayType); ok {
			if star, ok := arrType.Elt.(*ast.StarExpr); ok {
				if isSimpleSelector("embedded", "EmbeddedDir", star.X) {
					valid = true
				}
			}
		}
	}

	if !valid {
		return nil, []error{fmt.Errorf("not a []*embedded.EmbeddedDir: %#v", expr)}
	}
	for _, el := range lit.Elts {
		child, childErrors := parseDir(el, dirs, files)
		errors = append(errors, childErrors...)
		childDirs = append(childDirs, child)
	}
	return
}

func parseFilesSlice(expr ast.Expr, files map[string]*registeredFile) (childFiles []*registeredFile, errors []error) {
	valid := false
	lit, ok := expr.(*ast.CompositeLit)
	if ok {
		if arrType, ok := lit.Type.(*ast.ArrayType); ok {
			if star, ok := arrType.Elt.(*ast.StarExpr); ok {
				if isSimpleSelector("embedded", "EmbeddedFile", star.X) {
					valid = true
				}
			}
		}
	}

	if !valid {
		return nil, []error{fmt.Errorf("not a []*embedded.EmbeddedFile: %#v", expr)}
	}
	for _, el := range lit.Elts {
		child, childErrors := parseFile(el, files)
		errors = append(errors, childErrors...)
		childFiles = append(childFiles, child)
	}
	return
}

func parseDirsMap(expr ast.Expr, dirs map[string]*registeredDir, files map[string]*registeredFile) (childDirs map[string]*registeredDir, errors []error) {
	valid := false
	lit, ok := expr.(*ast.CompositeLit)
	if ok {
		if mapType, ok := lit.Type.(*ast.MapType); ok {
			if star, ok := mapType.Value.(*ast.StarExpr); ok {
				if isSimpleSelector("embedded", "EmbeddedDir", star.X) && isIdent("string", mapType.Key) {
					valid = true
				}
			}
		}
	}

	if !valid {
		return nil, []error{fmt.Errorf("not a map[string]*embedded.EmbeddedDir: %#v", expr)}
	}
	childDirs = make(map[string]*registeredDir)
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			errors = append(errors, fmt.Errorf("not a KeyValueExpr: %#v", el))
			continue
		}
		key, err := parseString(kv.Key)
		if err != nil {
			errors = append(errors, fmt.Errorf("key %s", err))
			continue
		}

		child, childErrors := parseDir(kv.Value, dirs, files)
		errors = append(errors, childErrors...)
		childDirs[key] = child
	}
	return
}

func parseFilesMap(expr ast.Expr, files map[string]*registeredFile) (childFiles map[string]*registeredFile, errors []error) {
	valid := false
	lit, ok := expr.(*ast.CompositeLit)
	if ok {
		if mapType, ok := lit.Type.(*ast.MapType); ok {
			if star, ok := mapType.Value.(*ast.StarExpr); ok {
				if isSimpleSelector("embedded", "EmbeddedFile", star.X) && isIdent("string", mapType.Key) {
					valid = true
				}
			}
		}
	}

	if !valid {
		return nil, []error{fmt.Errorf("not a map[string]*embedded.EmbeddedFile: %#v", expr)}
	}
	childFiles = make(map[string]*registeredFile)
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			errors = append(errors, fmt.Errorf("not a KeyValueExpr: %#v", el))
			continue
		}
		key, err := parseString(kv.Key)
		if err != nil {
			errors = append(errors, fmt.Errorf("key %s", err))
			continue
		}

		child, childErrors := parseFile(kv.Value, files)
		errors = append(errors, childErrors...)
		childFiles[key] = child
	}
	return
}

// unpoint returns the expression expr points to
// if expr is a & unary expression.
func unpoint(expr ast.Expr) ast.Expr {
	if expr, ok := expr.(*ast.UnaryExpr); ok {
		if expr.Op == token.AND {
			return expr.X
		}
	}
	return expr
}

func validateBox(t *testing.T, box *registeredBox, files []sourceFile) {
	dirsToBeChecked := make(map[string]struct{})
	filesToBeChecked := make(map[string]string)
	for _, file := range files {
		if !strings.HasPrefix(file.Name, box.Name) {
			continue
		}
		pathParts := strings.Split(file.Name, "/")
		dirs := pathParts[:len(pathParts)-1]
		dirPath := ""
		for _, dir := range dirs {
			if dir != box.Name {
				dirPath = path.Join(dirPath, dir)
			}
			dirsToBeChecked[dirPath] = struct{}{}
		}
		filesToBeChecked[path.Join(dirPath, pathParts[len(pathParts)-1])] = string(file.Contents)
	}

	if len(box.Files) != len(filesToBeChecked) {
		t.Errorf("box %v has incorrect number of files; expected %v, got %v", box.Name, len(filesToBeChecked), len(box.Files))
	}

	if len(box.Dirs) != len(dirsToBeChecked) {
		t.Errorf("box %v has incorrect number of dirs; expected %v, got %v", box.Name, len(dirsToBeChecked), len(box.Dirs))
	}

	for name, content := range filesToBeChecked {
		f, ok := box.Files[name]
		if !ok {
			t.Errorf("file %v not present in box %v", name, box.Name)
			continue
		}
		if f.Filename != name {
			t.Errorf("box %v: filename mismatch: key: %v; Filename: %v", box.Name, name, f.Filename)
		}
		if f.Content != content {
			t.Errorf("box %v: file %v content does not match: got %v, expected %v", box.Name, name, f.Content, content)
		}
		dirPath, _ := path.Split(name)
		dirPath = strings.TrimSuffix(dirPath, "/")
		dir, ok := box.Dirs[dirPath]
		if !ok {
			t.Errorf("directory %v not present in box %v", dirPath, box.Name)
			continue
		}
		found := false
		for _, file := range dir.ChildFiles {
			if file == f {
				found = true
			}
		}
		if !found {
			t.Errorf("file %v not found in directory %v in box %v", name, dirPath, box.Name)
			continue
		}
	}
	for name := range dirsToBeChecked {
		d, ok := box.Dirs[name]
		if !ok {
			t.Errorf("directory %v not present in box %v", name, box.Name)
			continue
		}
		if d.Filename != name {
			t.Errorf("box %v: filename mismatch: key: %v; Filename: %v", box.Name, name, d.Filename)
		}
		if name != "" {
			dirPath, _ := path.Split(name)
			dirPath = strings.TrimSuffix(dirPath, "/")
			dir, ok := box.Dirs[dirPath]
			if !ok {
				t.Errorf("directory %v not present in box %v", dirPath, box.Name)
				continue
			}
			found := false
			for _, dir := range dir.ChildDirs {
				if dir == d {
					found = true
				}
			}
			if !found {
				t.Errorf("directory %v not found in directory %v in box %v", name, dirPath, box.Name)
				continue
			}
		}
	}
}

func TestEmbedGo(t *testing.T) {
	sourceFiles := []sourceFile{
		{
			"boxes.go",
			[]byte(`package main

import (
	"github.com/GeertJohan/go.rice"
)

func main() {
	rice.MustFindBox("foo")
}
`),
		},
		{
			"foo/test1.txt",
			[]byte(`This is test 1`),
		},
		{
			"foo/test2.txt",
			[]byte(`This is test 2`),
		},
		{
			"foo/bar/test1.txt",
			[]byte(`This is test 1 in bar`),
		},
		{
			"foo/bar/baz/test1.txt",
			[]byte(`This is test 1 in bar/baz`),
		},
		{
			"foo/bar/baz/backtick`.txt",
			[]byte(`Backtick filename`),
		},
		{
			"foo/bar/baz/\"quote\".txt",
			[]byte(`double quoted filename`),
		},
		{
			"foo/bar/baz/'quote'.txt",
			[]byte(`single quoted filename`),
		},
		{
			"foo/`/`/`.txt",
			[]byte(`Backticks everywhere!`),
		},
		{
			"foo/new\nline",
			[]byte("File with newline in name. Yes, this is possible."),
		},
	}
	pkg, cleanup, err := setUpTestPkg("foobar", sourceFiles)
	defer cleanup()
	if err != nil {
		t.Error(err)
		return
	}

	var buffer bytes.Buffer

	err = writeBoxesGo(pkg, &buffer)
	if err != nil {
		t.Error(err)
		return
	}

	t.Logf("Generated file: \n%s", buffer.String())
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, "rice-box.go"), &buffer, 0)
	if err != nil {
		t.Error(err)
		return
	}

	var initFunc *ast.FuncDecl
	for _, decl := range f.Decls {
		if decl, ok := decl.(*ast.FuncDecl); ok && decl.Name != nil && decl.Name.Name == "init" {
			initFunc = decl
			break
		}
	}
	if initFunc == nil {
		t.Fatal("init function not found in generated file")
	}
	if initFunc.Body == nil {
		t.Fatal("init function has no body in generated file")
	}
	var registrations []*ast.CallExpr
	directories := make(map[string]*registeredDir)
	files := make(map[string]*registeredFile)
	_ = directories
	_ = files
	for _, stmt := range initFunc.Body.List {
		if stmt, ok := stmt.(*ast.ExprStmt); ok {
			if call, ok := stmt.X.(*ast.CallExpr); ok {
				registrations = append(registrations, call)
			}
			continue
		}
		if stmt, ok := stmt.(*ast.AssignStmt); ok {
			for i, rhs := range stmt.Rhs {
				// Rhs can be EmbeddedDir or EmbeddedFile.
				var literal *ast.CompositeLit
				literal, ok := unpoint(rhs).(*ast.CompositeLit)
				if !ok {
					continue
				}
				if lhs, ok := stmt.Lhs[i].(*ast.Ident); ok {
					// variable
					edir, direrrs := parseDir(literal, directories, files)
					efile, fileerrs := parseFile(literal, files)
					abort := false
					for _, err := range direrrs {
						t.Error("error while parsing dir: ", err)
						abort = true
					}
					for _, err := range fileerrs {
						t.Error("error while parsing file: ", err)
						abort = true
					}
					if abort {
						return
					}

					if edir == nil && efile == nil {
						continue
					}
					if edir != nil {
						directories[lhs.Name] = edir
					} else {
						files[lhs.Name] = efile
					}
				} else if lhs, ok := stmt.Lhs[i].(*ast.SelectorExpr); ok {
					selName, ok := getIdentName(lhs.Sel)
					if !ok || selName != "ChildDirs" {
						continue
					}
					varName, ok := getIdentName(lhs.X)
					if !ok {
						t.Fatalf("cannot parse ChildDirs assignment: %#v", lhs)
					}
					dir, ok := directories[varName]
					if !ok {
						t.Fatalf("variable %v not found", varName)
					}

					var errors []error
					dir.ChildDirs, errors = parseDirsSlice(rhs, directories, files)

					abort := false
					for _, err := range errors {
						t.Errorf("error parsing child dirs: %s", err)
						abort = true
					}
					if abort {
						return
					}
				}
			}
		}
	}
	if len(registrations) == 0 {
		t.Fatal("could not find registration of embedded box")
	}

	boxes := make(map[string]*registeredBox)

	for _, call := range registrations {
		if isSimpleSelector("embedded", "RegisterEmbeddedBox", call.Fun) {
			if len(call.Args) != 2 {
				t.Fatalf("incorrect arguments to embedded.RegisterEmbeddedBox: %#v", call.Args)
			}
			boxArg := unpoint(call.Args[1])
			name, err := parseString(call.Args[0])
			if err != nil {
				t.Fatalf("first argument to embedded.RegisterEmbeddedBox incorrect: %s", err)
			}
			boxLit, ok := boxArg.(*ast.CompositeLit)
			if !ok {
				t.Fatalf("second argument to embedded.RegisterEmbeddedBox is not a composite literal: %#v", boxArg)
			}
			abort := false
			box, errors := parseRegistration(boxLit, directories, files)
			for _, err := range errors {
				t.Error("error while parsing box: ", err)
				abort = true
			}
			if abort {
				return
			}
			if box == nil {
				t.Fatalf("second argument to embedded.RegisterEmbeddedBox is not an embedded.EmbeddedBox: %#v", boxArg)
			}
			if box.Name != name {
				t.Fatalf("first argument to embedded.RegisterEmbeddedBox is not the same as the name in the second argument: %v, %#v", name, boxArg)
			}
			boxes[name] = box
		}
	}

	// Validate that all boxes are present.
	if _, ok := boxes["foo"]; !ok {
		t.Error("box \"foo\" not found")
	}
	for _, box := range boxes {
		validateBox(t, box, sourceFiles)
	}
}

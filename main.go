package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"log"
	"os"

	"github.com/mewpkg/clog"
	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

func main() {
	var (
		output  string
		pkgPath string
		verbose bool
	)
	flag.StringVar(&output, "o", "", "output path")
	flag.StringVar(&pkgPath, "pkg", "github.com/jupiterrider/purego-sdl3/sdl", "package path")
	flag.BoolVar(&verbose, "v", false, "enable verbose debug output")
	flag.Parse()
	if !verbose {
		clog.SetPathLevel("main", clog.LevelWarn)
	}
	if err := genMethods(pkgPath, output); err != nil {
		log.Fatalf("%+v", err)
	}
}

type Gen struct {
	// package to analyze
	pkg *packages.Package
	// generated methods
	methods []*ast.FuncDecl
}

func genMethods(pkgPath, output string) error {
	pkg, err := loadPkg(pkgPath)
	if err != nil {
		return errors.WithStack(err)
	}
	gen := &Gen{
		pkg: pkg,
	}
	if err := gen.parsePkg(); err != nil {
		return errors.WithStack(err)
	}
	if err := gen.printMethods(output); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (gen *Gen) parsePkg() error {
	for _, file := range gen.pkg.Syntax {
		if err := gen.parseFile(file); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (gen *Gen) parseFile(file *ast.File) error {
	pos := gen.pkg.Fset.Position(file.FileStart)
	clog.Debugln("file:", pos.Filename)
	for _, decl := range file.Decls {
		if err := gen.parseDecl(decl); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (gen *Gen) parseDecl(decl ast.Decl) error {
	switch decl := decl.(type) {
	case *ast.GenDecl:
		//clog.Debugf("gen decl (%s): %#v", decl.Tok, decl)
	case *ast.FuncDecl:
		if err := gen.parseFuncDecl(decl); err != nil {
			return errors.WithStack(err)
		}
	default:
		panic(fmt.Errorf("support for declaration type %T not yet implemented", decl))
	}
	return nil
}

func (gen *Gen) parseFuncDecl(decl *ast.FuncDecl) error {
	if decl.Recv != nil {
		return nil // skip methods (already generated).
	}
	params := decl.Type.Params.List
	if len(params) == 0 {
		return nil // skip functions without parameters.
	}
	firstParam := params[0]
	if len(firstParam.Names) != 1 {
		// TODO: add support for `a, b T` parameter lists.
		return nil // skip `a, b T` parameter lists for now.
	}
	clog.Debugln("func:", decl.Name)
	firstParamName := firstParam.Names[0]
	firstParamType := gen.pkg.TypesInfo.Types[firstParam.Type].Type
	clog.Debugln("first param name:", firstParamName)
	clog.Debugln("first param type:", firstParamType)
	// if first parameter has valid type (e.g. *Window) convert to method.
	if !gen.isValidMethodType(firstParamType) {
		return nil // skip non-supported receiver type.
	}
	if err := gen.genMethod(decl); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (gen *Gen) genMethod(funcDecl *ast.FuncDecl) error {
	clog.Infoln("generating method:", funcDecl.Name)
	params := funcDecl.Type.Params.List
	firstParam := params[0]
	firstParamName := firstParam.Names[0]
	firstParamType := firstParam.Type
	funcName := funcDecl.Name.String()
	methodName := funcName
	if newMethodName, ok := renameMethod[funcName]; ok {
		methodName = newMethodName
	}
	doc := &ast.CommentGroup{}
	if funcDecl.Doc != nil {
		for _, comment := range funcDecl.Doc.List {
			newComment := &ast.Comment{
				Slash: 0,
				Text:  comment.Text,
			}
			doc.List = append(doc.List, newComment)
		}
	}
	methodDecl := &ast.FuncDecl{
		Doc: doc,
		Recv: &ast.FieldList{
			List: []*ast.Field{
				&ast.Field{
					Names: []*ast.Ident{
						ast.NewIdent(firstParamName.String()),
					},
					Type: firstParamType,
				},
			},
		},
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: funcDecl.Type.Params.List[1:], // skip first parameter (now receiver)
			},
			Results: funcDecl.Type.Results,
		},
	}
	var args []ast.Expr
	for _, paramField := range funcDecl.Type.Params.List {
		for _, paramName := range paramField.Names {
			arg := paramName
			args = append(args, arg)
		}
	}
	callExpr := &ast.CallExpr{
		Fun:  funcDecl.Name,
		Args: args,
	}
	hasReturn := funcDecl.Type.Results != nil && len(funcDecl.Type.Results.List) > 0
	var stmt ast.Stmt
	if hasReturn {
		stmt = &ast.ReturnStmt{
			Results: []ast.Expr{callExpr},
		}
	} else {
		stmt = &ast.ExprStmt{
			X: callExpr,
		}
	}
	methodDecl.Body = &ast.BlockStmt{
		List: []ast.Stmt{stmt},
	}
	gen.methods = append(gen.methods, methodDecl)
	return nil
}

const pre = `// Code generated by "genmethods"; DO NOT EDIT.
`

func (gen *Gen) printMethods(output string) error {
	file := &ast.File{
		Name: ast.NewIdent(gen.pkg.Name),
	}
	for _, method := range gen.methods {
		file.Decls = append(file.Decls, method)
	}
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, pre)
	if err := format.Node(buf, gen.pkg.Fset, file); err != nil {
		return errors.WithStack(err)
	}
	data, err := format.Source(buf.Bytes())
	if err != nil {
		return errors.WithStack(err)
	}
	if len(output) > 0 {
		clog.Debugf("writing to %q", output)
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return errors.WithStack(err)
		}
	} else {
		fmt.Print(string(data))
	}
	return nil
}

var validMethodTypes = map[string]bool{
	"*github.com/jupiterrider/purego-sdl3/sdl.Camera":   true,
	"*github.com/jupiterrider/purego-sdl3/sdl.Cursor":   true,
	"*github.com/jupiterrider/purego-sdl3/sdl.Renderer": true,
	"*github.com/jupiterrider/purego-sdl3/sdl.Surface":  true,
	"*github.com/jupiterrider/purego-sdl3/sdl.Texture":  true,
	"*github.com/jupiterrider/purego-sdl3/sdl.Window":   true,
}

var renameMethod = map[string]string{
	// Camera methods
	"AcquireCameraFrame": "AcquireFrame",
	"CloseCamera":        "Close",
	"ReleaseCameraFrame": "ReleaseFrame",
	// Cursor methods
	"DestroyCursor": "Destroy",
	// Renderer methods
	"GetRendererName":    "GetName",
	"DestroyRenderer":    "Destroy",
	"RenderClear":        "Clear",
	"RenderPresent":      "Present",
	"SetRenderDrawColor": "SetDrawColor",
	"SetRenderVSync":     "SetVSync",
	// Surface methods
	"BlitSurface":    "Blit",
	"DestroySurface": "Destroy",
	"LockSurface":    "Lock",
	"UnlockSurface":  "Unlock",
	// Texture methods
	"DestroyTexture": "Destroy",
	// Window methods
	"DestroyWindow":       "Destroy",
	"GetWindowSize":       "GetSize",
	"GetWindowSurface":    "GetSurface",
	"HideWindow":          "Hide",
	"SetWindowSize":       "SetSize",
	"ShowWindow":          "Show",
	"UpdateWindowSurface": "UpdateSurface",
}

func (gen *Gen) isValidMethodType(typ types.Type) bool {
	return validMethodTypes[typ.String()]
}

func loadPkg(pkgPath string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, pkg := range pkgs {
		if pkg.PkgPath == pkgPath {
			return pkg, nil
		}
	}
	return nil, errors.Errorf("unable to locate pkg %q in %#v", pkgs)
}

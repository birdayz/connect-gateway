package gengateway

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	commentWidth = 97

	generatedFilenameExtension = ".connect.gw.go"
	generatePackageSuffix      = "connect"
)

const (
	contextPackage        = protogen.GoImportPath("context")
	fmtPackage            = protogen.GoImportPath("fmt")
	connectGatewayPackage = protogen.GoImportPath("go.vallahaye.net/connect-gateway")
	runtimePackage        = protogen.GoImportPath("github.com/grpc-ecosystem/grpc-gateway/v2/runtime")
	codesPackage          = protogen.GoImportPath("google.golang.org/grpc/codes")
	statusPackage         = protogen.GoImportPath("google.golang.org/grpc/status")
)

func Generate(plugin *protogen.Plugin, file *protogen.File) {
	if len(file.Services) == 0 {
		return
	}
	file.GoPackageName += generatePackageSuffix
	generatedFilenamePrefixToSlash := filepath.ToSlash(file.GeneratedFilenamePrefix)
	file.GeneratedFilenamePrefix = path.Join(
		path.Dir(generatedFilenamePrefixToSlash),
		string(file.GoPackageName),
		path.Base(generatedFilenamePrefixToSlash),
	)
	generatedFile := plugin.NewGeneratedFile(
		file.GeneratedFilenamePrefix+generatedFilenameExtension,
		protogen.GoImportPath(path.Join(
			string(file.GoImportPath),
			string(file.GoPackageName),
		)),
	)
	generatedFile.Import(file.GoImportPath)
	generatePreamble(generatedFile, file)
	for _, service := range file.Services {
		generateService(generatedFile, file, service)
	}
}

func generatePreamble(g *protogen.GeneratedFile, file *protogen.File) {
	g.P("// Code generated by ", filepath.Base(os.Args[0]), ". DO NOT EDIT.")
	g.P("//")
	if file.Proto.GetOptions().GetDeprecated() {
		wrapComments(g, file.Desc.Path(), " is a deprecated file.")
	} else {
		g.P("// Source: ", file.Desc.Path())
	}
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()
}

func generateService(g *protogen.GeneratedFile, file *protogen.File, service *protogen.Service) {
	var (
		serviceHandlerGoName               = service.GoName + "Handler"
		serviceGatewayServerGoName         = service.GoName + "GatewayServer"
		newServiceGatewayServerGoName      = "New" + serviceGatewayServerGoName
		unimplementedServiceServerGoName   = fmt.Sprintf("Unimplemented%sServer", service.GoName)
		registerServiceGatewayServerGoName = fmt.Sprintf("Register%sHandlerGatewayServer", service.GoName)
		registerServiceServerGoName        = fmt.Sprintf("Register%sHandlerServer", service.GoName)
	)
	wrapComments(g, serviceGatewayServerGoName, " implements the gRPC server API for the ", service.GoName, " service.")
	if isDeprecatedService(service) {
		g.P("//")
		generateDeprecated(g)
	}
	g.P("type ", serviceGatewayServerGoName, " struct {")
	g.P(file.GoImportPath.Ident(unimplementedServiceServerGoName))
	for _, method := range service.Methods {
		if isUnaryMethod(method) {
			g.P(unexportedGoName(method.GoName), " ",
				connectGatewayPackage.Ident("UnaryHandler"), "[", method.Input.GoIdent, ", ", method.Output.GoIdent, "]")
		}
	}
	g.P("}")
	g.P()
	wrapComments(g, newServiceGatewayServerGoName, " constructs a Connect-Gateway gRPC server for the ", service.GoName, " service.")
	if isDeprecatedService(service) {
		g.P("//")
		generateDeprecated(g)
	}
	g.P("func ", newServiceGatewayServerGoName,
		"(svc ", serviceHandlerGoName, ", opts ...", connectGatewayPackage.Ident("HandlerOption"), ") *", serviceGatewayServerGoName, " {")
	g.P("return &", serviceGatewayServerGoName, "{")
	for _, method := range service.Methods {
		if isUnaryMethod(method) {
			var procedureName = fmt.Sprintf("/%s.%s/%s", method.Parent.Desc.ParentFile().Package(), method.Parent.Desc.Name(), method.Desc.Name())
			g.P(unexportedGoName(method.GoName), ": ",
				connectGatewayPackage.Ident("NewUnaryHandler"), `("`, procedureName, `", svc.`, method.GoName, ", opts...),")
		}
	}
	g.P("}")
	g.P("}")
	g.P()
	for _, method := range service.Methods {
		var methodServerGoName = fmt.Sprintf("%s_%sServer", service.GoName, method.GoName)
		if isUnaryMethod(method) {
			g.P("func (s *", serviceGatewayServerGoName, ") ", method.GoName,
				"(ctx ", contextPackage.Ident("Context"), ", req *", method.Input.GoIdent, ") (*", method.Output.GoIdent, ", error) {")
			g.P("return s.", unexportedGoName(method.GoName), "(ctx, req)")
			g.P("}")
		} else if !method.Desc.IsStreamingClient() {
			g.P("func (s *", serviceGatewayServerGoName, ") ", method.GoName,
				"(*", method.Input.GoIdent, ", ", file.GoImportPath.Ident(methodServerGoName), ") error {")
			generateStreamingNotSupported(g)
			g.P("}")
		} else {
			g.P("func (s *", serviceGatewayServerGoName, ") ", method.GoName, "(", file.GoImportPath.Ident(methodServerGoName), ") error {")
			generateStreamingNotSupported(g)
			g.P("}")
		}
		g.P()
	}
	wrapComments(g, registerServiceGatewayServerGoName, " registers the Connect handlers for the ", service.GoName, ` "svc" to "mux".`)
	if isDeprecatedService(service) {
		g.P("//")
		generateDeprecated(g)
	}
	g.P("func ", registerServiceGatewayServerGoName,
		"(mux *", runtimePackage.Ident("ServeMux"), ", svc ", serviceHandlerGoName, ", opts ...", connectGatewayPackage.Ident("HandlerOption"), ") {")
	g.P("if err := ", file.GoImportPath.Ident(registerServiceServerGoName),
		"(", contextPackage.Ident("TODO"), "(), mux, ", newServiceGatewayServerGoName, "(svc, opts...)); err != nil {")
	g.P("panic(", fmtPackage.Ident("Errorf"), `("connect-gateway: %w", err))`)
	g.P("}")
	g.P("}")
}

func generateStreamingNotSupported(g *protogen.GeneratedFile) {
	g.P("return ", statusPackage.Ident("Error"),
		"(", codesPackage.Ident("Unimplemented"), `, "streaming calls are not yet supported in the in-process transport")`)
}

func generateDeprecated(g *protogen.GeneratedFile) {
	g.P("// Deprecated: do not use.")
}

func isUnaryMethod(method *protogen.Method) bool {
	return !method.Desc.IsStreamingClient() && !method.Desc.IsStreamingServer()
}

func isDeprecatedService(service *protogen.Service) bool {
	serviceOptions, ok := service.Desc.Options().(*descriptorpb.ServiceOptions)
	return ok && serviceOptions.GetDeprecated()
}

func unexportedGoName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	r, size := utf8.DecodeRuneInString(name)
	b.WriteRune(unicode.ToLower(r))
	b.WriteString(name[size:])
	return b.String()
}

// Raggedy comments in the generated code are driving me insane. This
// word-wrapping function is ruinously inefficient, but it gets the job done.
//
// Source: https://github.com/bufbuild/connect-go/blob/main/cmd/protoc-gen-connect-go/main.go
func wrapComments(g *protogen.GeneratedFile, elems ...any) {
	text := &bytes.Buffer{}
	for _, el := range elems {
		switch el := el.(type) {
		case protogen.GoIdent:
			fmt.Fprint(text, g.QualifiedGoIdent(el))
		default:
			fmt.Fprint(text, el)
		}
	}
	words := strings.Fields(text.String())
	text.Reset()
	var pos int
	for _, word := range words {
		numRunes := utf8.RuneCountInString(word)
		if pos > 0 && pos+numRunes+1 > commentWidth {
			g.P("// ", text.String())
			text.Reset()
			pos = 0
		}
		if pos > 0 {
			text.WriteRune(' ')
			pos++
		}
		text.WriteString(word)
		pos += numRunes
	}
	if text.Len() > 0 {
		g.P("// ", text.String())
	}
}

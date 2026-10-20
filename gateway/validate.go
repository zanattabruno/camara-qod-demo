package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// specValidator rejects any request that does not match the CAMARA OpenAPI
// document, so the handlers only see well-formed input.
func specValidator(specPath string) (func(http.Handler) http.Handler, error) {
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		return nil, err
	}
	// Match on the path only, whatever host the client used.
	doc.Servers = openapi3.Servers{{URL: "/quality-on-demand/v1"}}
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, err
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, params, err := router.FindRoute(r)
			if err == routers.ErrPathNotFound {
				writeError(w, notFound())
				return
			} else if err != nil {
				writeError(w, invalid(err.Error()))
				return
			}
			in := &openapi3filter.RequestValidationInput{Request: r, PathParams: params, Route: route, Options: options}
			if err := openapi3filter.ValidateRequest(context.Background(), in); err != nil {
				writeError(w, invalid(firstLine(err.Error())))
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

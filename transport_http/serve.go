package transport_http

import (
	"context"
	"fmt"
	ctx "github.com/eden-framework/context"
	"github.com/profzone/envconfig"
	"net/http"
	_ "net/http/pprof"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/julienschmidt/httprouter"

	"github.com/eden-framework/courier"
)

type ServeHTTP struct {
	Name         string
	IP           string
	Port         int
	WriteTimeout envconfig.Duration
	ReadTimeout  envconfig.Duration
	WithCORS     bool
	router       *httprouter.Router
}

func (s ServeHTTP) MarshalDefaults(v interface{}) {
	if h, ok := v.(*ServeHTTP); ok {
		if h.Name == "" {
			h.Name = os.Getenv("PROJECT_NAME")
		}

		if h.Port == 0 {
			h.Port = 8000
		}

		if h.ReadTimeout == 0 {
			h.ReadTimeout = envconfig.Duration(15 * time.Second)
		}

		if h.WriteTimeout == 0 {
			h.WriteTimeout = envconfig.Duration(15 * time.Second)
		}
	}
}

func (s *ServeHTTP) Serve(wsCtx *ctx.WaitStopContext, router *courier.Router) error {
	s.MarshalDefaults(s)
	s.router = s.convertRouterToHttpRouter(router)
	s.router.GET("/healthz", func(http.ResponseWriter, *http.Request, httprouter.Params) {})

	srv := &http.Server{
		Handler:      s,
		Addr:         fmt.Sprintf("%s:%d", s.IP, s.Port),
		WriteTimeout: time.Duration(s.WriteTimeout),
		ReadTimeout:  time.Duration(s.ReadTimeout),
	}

	wsCtx.Add(1)
	go func() {
		<-wsCtx.Done()
		fmt.Println("HTTP server shutdown...")
		if err := srv.Shutdown(context.Background()); err != nil {
			fmt.Printf("HTTP server shutdown failed: %v", err)
		} else {
			fmt.Println("HTTP server shutdown complete.")
		}
		wsCtx.Finish()
	}()

	fmt.Printf("[Courier] HTTP listen on %s\n", srv.Addr)
	return srv.ListenAndServe()
}

var RxHttpRouterPath = regexp.MustCompile("/:([^/]+)")

func (s *ServeHTTP) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if s.WithCORS {
		headers := w.Header()
		setCORS(&headers)
	}
	s.router.ServeHTTP(w, req)
}

func (s *ServeHTTP) convertRouterToHttpRouter(router *courier.Router) *httprouter.Router {
	routes := router.Routes()

	if len(routes) == 0 {
		panic(fmt.Sprintf("need to register operation before Listion"))
	}

	r := httprouter.New()
	r.Handler(http.MethodGet, "/debug/pprof/", http.DefaultServeMux)
	r.Handler(http.MethodGet, "/debug/pprof/:item", http.DefaultServeMux)

	sort.Slice(routes, func(i, j int) bool {
		return getPath(routes[i]) < getPath(routes[j])
	})

	for _, route := range routes {
		method := getMethod(route)
		p := getPath(route)

		finalOperators, operatorTypeNames := route.EffectiveOperators()

		if len(finalOperators) == 0 {
			panic(fmt.Errorf(
				"[Courier] No available operator %v",
				route.Operators,
			))
		}

		if method == "" {
			panic(fmt.Errorf(
				"[Courier] Missing method of %s\n",
				color.CyanString(reflect.TypeOf(finalOperators[len(finalOperators)-1]).Name()),
			))
		}

		lengthOfOperatorTypes := len(operatorTypeNames)

		for i := range operatorTypeNames {
			if i < lengthOfOperatorTypes-1 {
				operatorTypeNames[i] = color.HiCyanString(operatorTypeNames[i])
			} else {
				operatorTypeNames[i] = color.HiMagentaString(operatorTypeNames[i])
			}
		}

		fmt.Printf(
			"[Courier] %s %s\n",
			colorByMethod(method)("%-8s %s", method, RxHttpRouterPath.ReplaceAllString(p, "/{$1}")),
			strings.Join(operatorTypeNames, " "),
		)

		r.Handle(method, p, CreateHttpHandler(s, finalOperators...))
	}

	return r
}

func getMethod(route *courier.Route) string {
	if withHttpMethod, ok := route.Operators[len(route.Operators)-1].(IMethod); ok {
		return string(withHttpMethod.Method())
	}
	return ""
}

func getPath(route *courier.Route) string {
	p := "/"
	for _, operator := range route.Operators {
		if WithHttpPath, ok := operator.(IPath); ok {
			p += WithHttpPath.Path()
		}
	}
	return httprouter.CleanPath(p)
}

func colorByMethod(method string) func(f string, args ...interface{}) string {
	switch method {
	case http.MethodGet:
		return color.BlueString
	case http.MethodPost:
		return color.GreenString
	case http.MethodPut:
		return color.YellowString
	case http.MethodDelete:
		return color.RedString
	case http.MethodHead:
		return color.WhiteString
	case http.MethodPatch:
		return color.MagentaString
	default:
		return color.BlackString
	}
}

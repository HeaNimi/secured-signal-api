package middlewares

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	loggerpkg "github.com/codeshelldev/gotl/pkg/logger"
	"github.com/codeshelldev/gotl/pkg/request"
	"github.com/codeshelldev/secured-signal-api/internals/config/structure"
	. "github.com/codeshelldev/secured-signal-api/internals/proxy/common"
	"github.com/codeshelldev/secured-signal-api/utils/logging"
)

var RequestLogger Middleware = Middleware{
	Name: "Logging",
	Use:  loggingHandler,
}

func loggingHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		logger := GetLogger(req)

		ip := GetContext[net.IP](req, ClientIPKey)

		decodedQuery, _ := url.QueryUnescape(req.URL.RawQuery)
		decodedQuery = strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(decodedQuery)

		logMessage := []any{
			ip.String(),
			" ",
			req.Method,
			" ",
			req.URL.Path,
			" ",
			decodedQuery,
		}

		shouldLogBody := req.Method == http.MethodPost && req.URL.Path == "/v2/send"
		if shouldLogBody {
			body, err := request.GetReqBody(req)

			if err == nil && body.Data != nil && !body.Empty {
				bodyJSON, err := json.Marshal(body.Data)

				if err == nil {
					logMessage = append(logMessage, " ", string(bodyJSON))
				}
			}
		}

		logger.Info(logMessage...)

		next.ServeHTTP(w, req)
	})
}

var InternalMiddlewareLogger Middleware = Middleware{
	Name: "_Middleware_Logger",
	Use:  middlewareLoggerHandler,
}

func middlewareLoggerHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conf := GetConfigWithoutDefaultByReq(req)

		var logLevel string

		if conf != nil && conf.TYPE != structure.MAIN {
			logLevel = conf.SERVICE.LOG_LEVEL
		}

		l := loggerpkg.Get()

		if strings.TrimSpace(logLevel) != "" {
			l = loggerpkg.Get().Sub(logLevel)

			transforms := logging.DefaultTransforms()
			transforms = append(transforms, func(content string) string {
				return conf.NAME + "\t" + content
			})

			l.SetTransform(logging.Apply(transforms...))
		} else {
			l.SetTransform(logging.Apply(logging.DefaultTransforms()...))
		}

		req = SetContext(req, LoggerKey, l)

		next.ServeHTTP(w, req)
	})
}

// Package httpapi exposes the same foundation services used by local CLI commands.
package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	json "encoding/json/v2"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type Executor interface {
	Execute(context.Context, string, app.Params) domain.Result
}
type Config struct {
	Host, Token, OwnerInstanceID string
	Assets                       fs.FS
}

func New(service Executor, config Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		mutationID := ""
		defer func() {
			if recover() != nil {
				if mutationID != "" {
					write(w, domain.Failure(mutationID, domain.Fail("write_uncertain", "The mutation acknowledgement is uncertain. Retry the same request identity and input.")))
					return
				}
				write(w, domain.Failure(domain.NewID(), domain.Fail("internal_error", "The operation could not complete. Report this request identity.")))
			}
		}()
		fail := func(code, message string) { write(w, domain.Failure(domain.NewID(), domain.Fail(code, message))) }
		if r.Host != config.Host {
			fail("invalid_host", "Use the exact local Council address.")
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			serveAsset(w, r, config.Assets)
			return
		}
		auth := r.Header.Values("Authorization")
		if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+config.Token)) != 1 {
			fail("unauthenticated", "Open Council using its local private launch URL.")
			return
		}
		origin := r.Header.Values("Origin")
		mutation := r.Method != "GET" && r.Method != "HEAD"
		if len(origin) > 1 || (len(origin) == 1 && origin[0] != "http://"+config.Host) || (mutation && len(origin) == 0) {
			fail("forbidden_origin", "Use the local Council interface for this operation.")
			return
		}
		if strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%2f") || strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%5c") {
			fail("invalid_request", "Use a documented API path.")
			return
		}
		op, id, allowed, keys := route(r.URL.Path, r.Method)
		if op == "" {
			fail("not_found", "This API route does not exist.")
			return
		}
		if !contains(strings.Split(allowed, ", "), r.Method) {
			w.Header().Set("Allow", allowed)
			fail("method_not_allowed", "Use the documented operation method.")
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			fail("invalid_request", "Use valid query parameters.")
			return
		}
		args := map[string]string{}
		for key, values := range query {
			if len(values) != 1 || values[0] == "" || !contains(keys, key) {
				fail("invalid_request", "Use documented, nonduplicate query parameters.")
				return
			}
			args[key] = values[0]
		}
		var body []byte
		if mutation {
			media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			validParams := true
			for name := range params {
				if name != "charset" {
					validParams = false
				}
			}
			if !validParams || err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) || len(params) > 1 {
				fail("unsupported_media_type", "Send application/json with UTF-8 encoding.")
				return
			}
			body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, domain.MaxBodyBytes))
			if err != nil {
				fail("input_too_large", "The request body exceeds 32 MiB or could not be read. Reduce it explicitly.")
				return
			}
			var object map[string]any
			if e := domain.StrictDecode(body, &object); e != nil {
				write(w, domain.Failure(domain.NewID(), e))
				return
			}
			if requestID, ok := object["requestId"].(string); ok && domain.ValidID(requestID) {
				mutationID = requestID
			}
		} else if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			fail("invalid_request", "GET requests must not contain a body.")
			return
		}
		if op == "meta" {
			metadata := map[string]any{"scope": "workspace_foundation", "managedExecution": "not_implemented"}
			if provider, ok := service.(interface {
				Metadata(context.Context) (map[string]any, error)
			}); ok {
				var err error
				metadata, err = provider.Metadata(r.Context())
				if err != nil {
					write(w, domain.Failure(domain.NewID(), domain.SafeError(err)))
					return
				}
			}
			metadata["appVersion"] = domain.AppVersion
			metadata["apiVersion"] = domain.APIVersion
			metadata["ownerInstanceId"] = config.OwnerInstanceID
			write(w, domain.Success(200, domain.NewID(), metadata, nil))
			return
		}
		write(w, service.Execute(r.Context(), op, app.Params{ID: id, Query: args, Body: body}))
	})
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func route(p, requestedMethod string) (operation, id, method string, queries []string) {
	if op, identity, methods, keys := executionRoute(p, requestedMethod); op != "" {
		return op, identity, methods, keys
	}
	switch p {
	case "/api/v1/meta":
		return "meta", "", "GET", nil
	case "/api/v1/projects/open":
		return "projects.open", "", "POST", nil
	case "/api/v1/projects":
		return "projects.list", "", "GET", []string{"limit", "cursor"}
	case "/api/v1/agents":
		return "agents.list", "", "GET", nil
	case "/api/v1/policy":
		if requestedMethod == "PUT" {
			return "policy.set", "", "GET, PUT", nil
		}
		return "policy.get", "", "GET, PUT", nil
	case "/api/v1/drafts":
		if requestedMethod == "POST" {
			return "drafts.create", "", "GET, POST", nil
		}
		return "drafts.list", "", "GET, POST", []string{"projectId", "limit", "cursor"}
	}
	for _, resource := range []string{"projects", "drafts"} {
		prefix := "/api/v1/" + resource + "/"
		if strings.HasPrefix(p, prefix) {
			id = strings.TrimPrefix(p, prefix)
			if id == "" || strings.Contains(id, "/") {
				return "", "", "", nil
			}
			if resource == "drafts" {
				if requestedMethod == "PUT" {
					return "drafts.update", id, "GET, PUT", nil
				}
				return "drafts.get", id, "GET, PUT", []string{"revision"}
			}
			return "projects.get", id, "GET", nil
		}
	}
	return "", "", "", nil
}

func write(w http.ResponseWriter, result domain.Result) {
	body, err := json.Marshal(result.Envelope)
	if err != nil {
		result = domain.Failure(domain.NewID(), domain.Fail("internal_error", "The response could not be encoded. Report this request identity."))
		body, _ = json.Marshal(result.Envelope)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(result.Status)
	_, _ = w.Write(append(body, '\n'))
}

func serveAsset(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return
	}
	if assets == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || path.Clean(name) != name {
		http.NotFound(w, r)
		return
	}
	switch path.Ext(name) {
	case ".html", ".js", ".css", ".svg":
	default:
		http.NotFound(w, r)
		return
	}
	body, err := fs.ReadFile(assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if kind := mime.TypeByExtension(path.Ext(name)); kind != "" {
		w.Header().Set("Content-Type", kind)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
}

package cli

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type command struct {
	name, op, method, route string
	json, noOpen            bool
	params                  app.Params
	requestID               string
}

func parse(args []string, stdin io.Reader) (command, *domain.Error) {
	c := command{params: app.Params{Query: map[string]string{}}}
	flags := map[string]string{}
	for _, argument := range args {
		if argument == "--json" {
			c.json = true
		}
	}
	positionals := []string{}
	bools := map[string]bool{"json": true, "no-open": true, "use-current-policy": true, "help": true, "version": true}
	values := map[string]bool{"api-version": true, "request-id": true, "expected-revision": true, "limit": true, "cursor": true, "project": true, "task": true, "task-file": true, "revision": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		key := strings.TrimPrefix(arg, "--")
		if _, exists := flags[key]; exists {
			return c, domain.Invalid("nonduplicate command options")
		}
		if bools[key] {
			flags[key] = "true"
			continue
		}
		if !values[key] || i+1 >= len(args) {
			return c, domain.Invalid("documented command options and values")
		}
		i++
		flags[key] = args[i]
	}
	c.json = flags["json"] == "true"
	c.noOpen = flags["no-open"] == "true"
	for key, value := range flags {
		if values[key] && value == "" {
			return c, domain.Invalid("nonempty option values")
		}
	}
	if api, ok := flags["api-version"]; ok && api != "1" {
		return c, domain.Invalid("API version 1")
	}
	if flags["help"] != "" || len(args) == 0 {
		c.name = "help"
		return c, nil
	}
	if flags["version"] != "" {
		for key := range flags {
			if key != "version" && key != "json" && key != "api-version" {
				return c, domain.Invalid("version command options")
			}
		}
		if len(positionals) != 0 {
			return c, domain.Invalid("version command")
		}
		c.name = "version"
		return c, nil
	}
	if len(positionals) == 0 {
		return c, domain.Invalid("command")
	}
	c.name = strings.Join(positionals[:min(2, len(positionals))], " ")
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	need := func(key string) bool { v, ok := flags[key]; return ok && v != "" }
	var body map[string]any
	switch c.name {
	case "ui":
		if len(positionals) != 1 {
			return c, domain.Invalid("ui command")
		}
		allow("no-open")
	case "projects open":
		if len(positionals) != 3 {
			return c, domain.Invalid("project path")
		}
		allow("request-id")
		base, err := os.Getwd()
		if err != nil {
			return c, domain.Fail("project_unavailable", "Cannot resolve the current working directory.")
		}
		c.op = "projects.open"
		c.method = "POST"
		c.route = "/projects/open"
		body = map[string]any{"selectedPath": positionals[2], "basePath": base}
	case "projects list", "drafts list":
		if len(positionals) != 2 {
			return c, domain.Invalid("list command")
		}
		allow("limit", "cursor")
		resource := positionals[0]
		c.op = resource + ".list"
		c.method = "GET"
		c.route = "/" + resource
		if resource == "drafts" {
			allow("project")
			if need("project") {
				c.params.Query["projectId"] = flags["project"]
			}
		}
		for _, key := range []string{"limit", "cursor"} {
			if v, ok := flags[key]; ok {
				if v == "" {
					return c, domain.Invalid(key)
				}
				c.params.Query[key] = v
			}
		}
	case "projects get", "drafts get":
		if len(positionals) != 3 || !domain.ValidID(positionals[2]) {
			return c, domain.Invalid("resource identity")
		}
		c.params.ID = positionals[2]
		c.op = positionals[0] + ".get"
		c.method = "GET"
		c.route = "/" + positionals[0] + "/" + positionals[2]
		if positionals[0] == "drafts" {
			allow("revision")
			if need("revision") {
				c.params.Query["revision"] = flags["revision"]
			}
		}
	case "policy get":
		if len(positionals) != 2 {
			return c, domain.Invalid("policy command")
		}
		c.op = "policy.get"
		c.method = "GET"
		c.route = "/policy"
	case "policy set":
		if len(positionals) != 3 || !need("expected-revision") || (positionals[2] != "autonomous" && positionals[2] != "supervised") {
			return c, domain.Invalid("policy mode and expected revision")
		}
		allow("expected-revision", "request-id")
		c.op = "policy.set"
		c.method = "PUT"
		c.route = "/policy"
		body = map[string]any{"mode": positionals[2], "expectedRevision": flags["expected-revision"]}
	case "agents list", "agents refresh":
		if len(positionals) != 2 {
			return c, domain.Invalid("agents command")
		}
		c.op = "agents.list"
		c.method = "GET"
		c.route = "/agents"
	case "drafts create", "drafts update":
		allow("project", "task", "task-file", "request-id")
		_, hasText := flags["task"]
		_, hasFile := flags["task-file"]
		if hasText == hasFile {
			return c, domain.Invalid("exactly one of --task or --task-file")
		}
		text := flags["task"]
		if hasFile {
			reader := stdin
			var file *os.File
			if flags["task-file"] != "-" {
				var err error
				file, err = os.Open(flags["task-file"])
				if err != nil {
					return c, domain.Fail("prerequisite_missing", "Cannot read the task file. Check its path and access.")
				}
				defer file.Close()
				reader = file
			}
			if reader == nil {
				return c, domain.Invalid("task input")
			}
			raw, err := io.ReadAll(io.LimitReader(reader, domain.MaxTextBytes+1))
			if err != nil {
				return c, domain.Invalid("readable task input")
			}
			if len(raw) > domain.MaxTextBytes {
				return c, domain.Fail("input_too_large", "Task text exceeds 4 MiB. Reduce it explicitly and submit again.")
			}
			if !utf8.Valid(raw) {
				return c, domain.Fail("invalid_encoding", "Supply a valid UTF-8 task file without replacement decoding.")
			}
			text = string(raw)
		}
		if err := domain.ValidateText(text); err != nil {
			return c, err
		}
		body = map[string]any{"originalText": text}
		if c.name == "drafts create" {
			if len(positionals) != 2 || !need("project") {
				return c, domain.Invalid("draft project")
			}
			body["projectId"] = flags["project"]
			c.op = "drafts.create"
			c.method = "POST"
			c.route = "/drafts"
		} else {
			allow("expected-revision", "use-current-policy")
			delete(allowed, "project")
			if len(positionals) != 3 || !domain.ValidID(positionals[2]) || !need("expected-revision") {
				return c, domain.Invalid("draft identity and expected revision")
			}
			c.params.ID = positionals[2]
			body["expectedRevision"] = flags["expected-revision"]
			if flags["use-current-policy"] != "" {
				body["useCurrentPolicy"] = true
			}
			c.op = "drafts.update"
			c.method = "PUT"
			c.route = "/drafts/" + positionals[2]
		}
	default:
		return c, domain.Invalid("documented Council command")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this command")
		}
	}
	if revision, ok := flags["expected-revision"]; ok {
		if _, err := domain.ParseRevision(revision); err != nil {
			return c, err
		}
	}
	if revision, ok := flags["revision"]; ok {
		if _, err := domain.ParseRevision(revision); err != nil {
			return c, err
		}
	}
	if limit, ok := flags["limit"]; ok {
		if _, err := domain.ParseLimit(limit); err != nil {
			return c, err
		}
	}
	if body != nil {
		c.requestID = flags["request-id"]
		if c.requestID == "" {
			c.requestID = domain.NewID()
		}
		if !domain.ValidID(c.requestID) {
			return c, domain.Invalid("request identity")
		}
		body["requestId"] = c.requestID
		encoded, err := json.Marshal(body)
		if err != nil {
			return c, domain.Invalid("Unicode request input")
		}
		c.params.Body = encoded
	}
	return c, nil
}

func (c command) queryString() string {
	q := url.Values{}
	for k, v := range c.params.Query {
		q.Set(k, v)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func exitCode(code string) int {
	switch domain.Status(code) {
	case 400, 413, 415:
		return 2
	case 404, 422:
		return 3
	case 409:
		return 4
	case 401, 403:
		return 5
	case 503:
		if code == "owner_unavailable" {
			return 5
		}
	}
	return 1
}

func human(result domain.Result) string {
	if result.Envelope.Error != nil {
		text := result.Envelope.Error.Message + " [" + result.Envelope.Error.Code + "] Request: " + result.Envelope.RequestID
		if result.Envelope.Error.Code == "write_uncertain" {
			text += ". Retry the same command with --request-id " + result.Envelope.RequestID
		}
		return text + "\n"
	}
	encoded, err := json.Marshal(result.Envelope.Data, jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		return "Operation completed.\n"
	}
	return string(encoded) + "\n"
}
